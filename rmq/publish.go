package rmq

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"log"

	"github.com/streadway/amqp"
)

const TopicExchangeName = "amq.topic"
const DelayExchangeName = "door.delay"

type RMQ struct {
	publishChannel   *amqp.Channel
	consumChannel    *amqp.Channel
	conn             *amqp.Connection
	replyMap         map[string]*amqp.Queue
	mutex            *sync.Mutex
	rabbitCloseError chan *amqp.Error
	amqpUri          string
	consumeHandlers  map[string]RMQConsumer
	UseQos           bool
}

type RMQConsumer struct {
	consumeType                   int //1.consumer 2.pull
	exchange, queueName, bindKeys string
	autoAck                       bool
	goroutineCnt                  int
	rmqHandler                    RMQHandler
	deliveryHandler               DeliveryHandler
}

func failOnError(err error, msg string) {
	if err != nil {
		log.Printf("%s: %s", msg, err)
	}
}

func (rmq *RMQ) connectToRabbitMQ(uri string) {
	for {
		conn, err := amqp.Dial(uri)
		if err != nil {
			log.Println("链接rabbit-mq 异常 地址:" + uri + err.Error())
			time.Sleep(time.Second * 1)
			continue
		}
		rmq.conn = conn

		// publish channel
		ch, err := conn.Channel()
		if err != nil {
			log.Println("链接rabbit-mq publish channel 异常 地址:" + uri + err.Error())
			time.Sleep(time.Second * 1)
			continue
		}
		rmq.publishChannel = ch

		// //尝试定义delay exchange，成功就成功，失败就失败
		// rmq.publishChannel.ExchangeDeclare(DelayExchangeName, "x-delayed-message",
		// 	true, false, false, false,
		// 	amqp.Table{
		// 		"x-delayed-type": "direct",
		// 	})

		//consume channel
		ch2, err := conn.Channel()
		if err != nil {
			log.Println("链接rabbit-mq  consume channel 异常 地址:" + uri + err.Error())
			time.Sleep(time.Second * 1)
			continue
		}
		rmq.consumChannel = ch2

		if err == nil {
			break
		}
	}
}

func (rmq *RMQ) rabbitConnector() {
	if perr := recover(); perr != nil {
		log.Println(fmt.Sprintf("error %v", perr))
	}
	var rabbitErr *amqp.Error

	for {
		log.Printf("start to %s\n", rmq.amqpUri)
		rabbitErr = <-rmq.rabbitCloseError
		log.Printf("recoonnect to %s %v\n", rmq.amqpUri, rabbitErr)
		reconnectedChan := make(chan bool)
		go Reconnect2RMQ(rabbitErr, rmq, reconnectedChan)
		select {
		case <-time.After(time.Second * 10):
			panic("restarted")
		case <-reconnectedChan:
			continue
		}
	}
}
func Reconnect2RMQ(rabbitErr *amqp.Error, rmq *RMQ, retch chan bool) {
	if rabbitErr != nil {
		rmq.publishChannel.Close()
		rmq.consumChannel.Close()
		rmq.conn.Close()

		rmq.connectToRabbitMQ(rmq.amqpUri)
		rmq.rabbitCloseError = make(chan *amqp.Error)
		rmq.conn.NotifyClose(rmq.rabbitCloseError)

		// run your setup process here
		for _, consumer := range rmq.consumeHandlers {
			if consumer.consumeType == 1 {
				if rmq.UseQos {
					rmq.RMQConsumeWithExchangeAndGoroutineAndQos(consumer.exchange, consumer.queueName, consumer.bindKeys, consumer.autoAck, consumer.goroutineCnt, consumer.rmqHandler)
				} else {
					rmq.RMQConsumeWithExchangeAndGoroutine(consumer.exchange, consumer.queueName, consumer.bindKeys, consumer.autoAck, consumer.goroutineCnt, consumer.rmqHandler)

				}
			} else if consumer.consumeType == 2 {
				rmq.RMQPullWithGoroutine(consumer.queueName, consumer.bindKeys, consumer.autoAck, consumer.goroutineCnt, consumer.rmqHandler)
			} else if consumer.consumeType == 3 {
				rmq.ConsumeWithDelivery(consumer.queueName, consumer.bindKeys, consumer.autoAck, consumer.goroutineCnt, consumer.deliveryHandler)
			}
		}
		retch <- true
	}
}

func New(addr string) *RMQ {

	if addr == "" {
		panic("初始化rmq失败，缺少参数rmq_address")
	}

	if !strings.HasSuffix(addr, "/") {
		addr = addr + "/"
	}

	return NewWithVhost(addr)
}

func NewWithVhost(rmqBaseAddr string) *RMQ {
	rmq := newrmq(rmqBaseAddr)
	return rmq
}

func newrmq(host string) *RMQ {

	rmq := &RMQ{
		replyMap:         make(map[string]*amqp.Queue, 0),
		mutex:            new(sync.Mutex),
		consumeHandlers:  make(map[string]RMQConsumer),
		amqpUri:          host,
		rabbitCloseError: make(chan *amqp.Error),
	}
	rmq.connectToRabbitMQ(rmq.amqpUri)
	rmq.conn.NotifyClose(rmq.rabbitCloseError)
	go rmq.rabbitConnector()
	return rmq
}

func (rmq *RMQ) Destory() {
	rmq.publishChannel.Close()
	rmq.consumChannel.Close()
	rmq.conn.Close()
}

func (rmq *RMQ) PublishWithUid(ctx context.Context, uid uint64, key string, body interface{}) error {
	headers := amqp.Table{}
	headers[RMQ_HEADER_USER_ID_KEY] = strconv.FormatUint(uid, 10)
	return rmq.PublishWithExchangeAndHeaders(ctx, TopicExchangeName, key, headers, body)
}

func (rmq *RMQ) Publish(ctx context.Context, key string, body interface{}) error {
	return rmq.PublishWithExchangeAndHeaders(ctx, TopicExchangeName, key, amqp.Table{}, body)
	// defer func() {
	// 	if perr := recover(); perr != nil {
	// 		log.Println(string(trace.PanicTrace(10)))
	// 	}
	// }()

	// jsonStr, err := json.Marshal(body)
	// if err != nil {
	// 	return err
	// }

	// err = rmq.publishChannel.Publish(
	// 	TopicExchangeName,
	// 	key,
	// 	false,
	// 	false,
	// 	amqp.Publishing{
	// 		ContentType: "text/plain",
	// 		Body:        jsonStr,
	// 	},
	// )
	// if err != nil {
	// 	return err
	// }
	// return nil

}

func (rmq *RMQ) PublishV2(ctx context.Context, key string, body interface{}) error {
	defer func() {
		if perr := recover(); perr != nil {
			log.Println(perr)
		}
	}()

	jsonStr, err := json.Marshal(body)
	if err != nil {
		return err
	}

	err = rmq.publishChannel.Publish(
		TopicExchangeName,
		key,
		false,
		false,
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        jsonStr,
		},
	)
	if err != nil {
		return err
	}
	return nil

}

const (
	RMQ_HEADER_TRACE_ID_KEY     = "_trace_id"
	RMQ_HEADER_PREV_APPNAME_KEY = "_prev_appname"
	RMQ_HEADER_PREV_METHOD_KEY  = "_prev_method"
	RMQ_HEADER_USER_ID_KEY      = "_user_id"
)

var lock = sync.Mutex{}

func (rmq *RMQ) PublishWithExchangeAndHeaders(ctx context.Context, exchange, key string, headers amqp.Table, body interface{}) (err error) {
	defer func() {
		if perr := recover(); perr != nil {
			// ignore ding message
			err = fmt.Errorf("%#v %#v", err, perr)
		}
	}()

	headers[RMQ_HEADER_PREV_METHOD_KEY] = key
	jsonStr, err := json.Marshal(body)
	if err != nil {
		return err
	}

	pub := &amqp.Publishing{
		Headers:     headers,
		ContentType: "text/json",
		Body:        jsonStr,
	}

	lock.Lock()
	defer lock.Unlock()
	err = rmq.publishChannel.Publish(
		exchange,
		key,
		false,
		false,
		*pub,
	)
	if err != nil {
		return err
	}

	return nil
}

func (rmq *RMQ) PublishDelayMessageWithUid(ctx context.Context, uid uint64, key string, delay int64, body interface{}) error {
	headers := amqp.Table{"x-delay": delay}
	headers[RMQ_HEADER_USER_ID_KEY] = strconv.FormatUint(uid, 10)
	return rmq.PublishWithExchangeAndHeaders(ctx, DelayExchangeName, key, headers, body)
}

func (rmq *RMQ) PublishWithUserID(ctx context.Context, uid string, key string, body interface{}) error {
	headers := amqp.Table{}
	headers[RMQ_HEADER_USER_ID_KEY] = uid
	return rmq.PublishWithExchangeAndHeaders(ctx, TopicExchangeName, key, headers, body)
}

func (rmq *RMQ) PublishDelayMessageWithUserID(ctx context.Context, uid string, key string, delay int64, body interface{}) error {
	headers := amqp.Table{"x-delay": delay}
	headers[RMQ_HEADER_USER_ID_KEY] = uid
	return rmq.PublishWithExchangeAndHeaders(ctx, DelayExchangeName, key, headers, body)
}
