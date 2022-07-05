package rmq

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/streadway/amqp"
)

/*
 * 处理消息的handler，输入是消息体的[]byte
 * 返回值
 * ack 是否确认ack，如果为true直接进行ack，否则进行nack并且进行重新投递
 * 		如果不需要重新处理，请返回false
 * out 这里返回值并没有业务意义，可以被系统记录到日志系统里面
 *
 */
type RMQHandler func(body []byte) (ack bool, err error)
type DeliveryHandler func(d *amqp.Delivery) (ack bool, err error)

func (rmq *RMQ) RMQConsume(queueName, bindKeys string, autoAck bool, rmqHandler RMQHandler) {
	rmq.RMQConsumeWithGoroutine(queueName, bindKeys, autoAck, 1, rmqHandler)
}

func (rmq *RMQ) RMQConsumeWithGoroutine(queueName, bindKeys string, autoAck bool, goroutineCnt int, rmqHandler RMQHandler) {
	rmq.RMQConsumeWithExchangeAndGoroutine("amq.topic", queueName, bindKeys, autoAck, goroutineCnt, rmqHandler)
}

func (rmq *RMQ) RMQConsumeWithGoroutineAndQos(queueName, bindKeys string, autoAck bool, goroutineCnt int, rmqHandler RMQHandler) {
	rmq.RMQConsumeWithExchangeAndGoroutineAndQos("amq.topic", queueName, bindKeys, autoAck, goroutineCnt, rmqHandler)
}

func (rmq *RMQ) RMQConsumeWithExchangeAndGoroutine(exchange, queueName, bindKeys string, autoAck bool, goroutineCnt int, rmqHandler RMQHandler) {
	//ch, err := rmq.conn.Channel()
	//failOnError(err, "Failed to open a channel")
	rmq.mutex.Lock()
	defer rmq.mutex.Unlock()
	rmq.consumeHandlers[queueName] = RMQConsumer{
		consumeType:  1,
		queueName:    queueName,
		bindKeys:     bindKeys,
		exchange:     exchange,
		autoAck:      autoAck,
		goroutineCnt: goroutineCnt,
		rmqHandler:   rmqHandler,
	}

	rmq.consumChannel.Qos(goroutineCnt, 0, false)

	q, err := rmq.consumChannel.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when usused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	failOnError(err, "Failed to QueueDeclare")

	for _, bindKey := range strings.Split(bindKeys, ",") {
		err := rmq.consumChannel.QueueBind(
			q.Name,   // queue name
			bindKey,  // routing key
			exchange, // exchange
			false,
			nil)
		failOnError(err, fmt.Sprintf("Failed to bind a queue,%s", queueName))
	}
	msgs, err := rmq.consumChannel.Consume(
		queueName, // queue
		"",        // consumer
		autoAck,   // auto ack
		false,     // exclusive
		false,     // no local
		false,     // no wait
		nil,       // args
	)

	failOnError(err, "Failed to new a Consume")
	for i := 0; i < goroutineCnt; i++ {
		go func() {
			for d := range msgs {
				// log.Printf("Received a message: %s", d.Body)
				ack, err := handlerRMQMsg(autoAck, d, queueName, rmqHandler)
				if err != nil {
					log.Println(ack)
					log.Println(err)
				}

			}
		}()
	}

}

func (rmq *RMQ) RMQConsumeWithExchangeAndGoroutineAndQos(exchange, queueName, bindKeys string, autoAck bool, goroutineCnt int, rmqHandler RMQHandler) {
	//ch, err := rmq.conn.Channel()
	//failOnError(err, "Failed to open a channel")
	rmq.mutex.Lock()
	defer rmq.mutex.Unlock()
	rmq.consumeHandlers[queueName] = RMQConsumer{
		consumeType:  1,
		queueName:    queueName,
		bindKeys:     bindKeys,
		exchange:     exchange,
		autoAck:      autoAck,
		goroutineCnt: goroutineCnt,
		rmqHandler:   rmqHandler,
	}

	rmq.consumChannel.Qos(goroutineCnt, 0, false)

	q, err := rmq.consumChannel.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when usused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	failOnError(err, "Failed to QueueDeclare")

	for _, bindKey := range strings.Split(bindKeys, ",") {
		err := rmq.consumChannel.QueueBind(
			q.Name,   // queue name
			bindKey,  // routing key
			exchange, // exchange
			false,
			nil)
		failOnError(err, fmt.Sprintf("Failed to bind a queue,%s", queueName))
	}
	msgs, err := rmq.consumChannel.Consume(
		queueName, // queue
		"",        // consumer
		autoAck,   // auto ack
		false,     // exclusive
		false,     // no local
		false,     // no wait
		nil,       // args
	)

	failOnError(err, "Failed to new a Consume")
	for i := 0; i < goroutineCnt; i++ {
		go func() {
			for d := range msgs {
				handlerRMQMsg(autoAck, d, queueName, rmqHandler)
			}
		}()
	}

}

func handlerRMQMsg(autoAck bool, d amqp.Delivery, queueName string, rmqHandler RMQHandler) (ack bool, err error) {
	return handlerRMQMsgWithDeliveryHandler(autoAck, d, queueName, func(d *amqp.Delivery) (ack bool, err error) {
		return rmqHandler(d.Body)
	})
}

func handlerRMQMsgWithDeliveryHandler(autoAck bool, d amqp.Delivery, queueName string, deliveryHandler DeliveryHandler) (ack bool, err error) {
	defer func() {
		if perr := recover(); perr != nil {
			err = fmt.Errorf("panic %v", perr)
		}
	}()
	defer func() {
		if !autoAck {
			if ack {
				d.Ack(false)
			} else {
				d.Nack(false, true)
			}
		}
	}()

	return deliveryHandler(&d)
}

func (rmq *RMQ) RMQPullWithGoroutine(queueName, bindKeys string, autoAck bool, goroutineCnt int, rmqHandler RMQHandler) {
	ch, err := rmq.conn.Channel()
	failOnError(err, "Failed to open a channel")

	rmq.mutex.Lock()
	defer rmq.mutex.Unlock()
	rmq.consumeHandlers[queueName] = RMQConsumer{
		consumeType:  2,
		queueName:    queueName,
		bindKeys:     bindKeys,
		exchange:     "amq.topic",
		autoAck:      autoAck,
		goroutineCnt: goroutineCnt,
		rmqHandler:   rmqHandler,
	}

	q, err := ch.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when usused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	for _, bindKey := range strings.Split(bindKeys, ",") {
		err = ch.QueueBind(
			q.Name,      // queue name
			bindKey,     // routing key
			"amq.topic", // exchange
			false,
			nil)
		failOnError(err, "Failed to bind a queue")
	}

	failOnError(err, "Failed to new a Consume")
	for i := 0; i < goroutineCnt; i++ {
		go func() {
			for {
				d, ok, err := ch.Get(queueName, autoAck)
				if err == amqp.ErrClosed {
					return
				}
				if err != nil {
					log.Printf("拉去消息异常 %#v\n", err)
				}
				if !ok {
					time.Sleep(time.Second * 1)
					continue
				}
				handlerRMQMsg(autoAck, d, queueName, rmqHandler)
			}
		}()
	}

}

func (rmq *RMQ) declareQueueThenBindKeysAndExchanges(queueName string, keys []string, exchanges []string) error {
	q, err := rmq.consumChannel.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when usused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		return err
	}

	for _, key := range keys {
		for _, exchange := range exchanges {
			err := rmq.consumChannel.QueueBind(
				q.Name,   // queue name
				key,      // routing key
				exchange, // exchange
				false,
				nil)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (rmq *RMQ) ConsumeWithDelivery(queueName, bindKeys string, autoAck bool, goroutineCnt int, deliveryHandler DeliveryHandler) {
	rmq.mutex.Lock()
	defer rmq.mutex.Unlock()

	// 重连后需要用的
	rmq.consumeHandlers[queueName] = RMQConsumer{
		consumeType:     3,
		queueName:       queueName,
		bindKeys:        bindKeys,
		autoAck:         autoAck,
		goroutineCnt:    goroutineCnt,
		deliveryHandler: deliveryHandler,
	}

	// 定义和绑定
	keys := strings.Split(bindKeys, ",")
	err := rmq.declareQueueThenBindKeysAndExchanges(queueName, keys, []string{TopicExchangeName, DelayExchangeName})
	failOnError(err, "declareQueueThenBindKeysAndExchanges failed")

	rmq.consumChannel.Qos(goroutineCnt, 0, false)

	msgs, err := rmq.consumChannel.Consume(
		queueName, // queue
		"",        // consumer
		autoAck,   // auto ack
		false,     // exclusive
		false,     // no local
		false,     // no wait
		nil,       // args
	)
	failOnError(err, "Failed to new a Consume")

	for i := 0; i < goroutineCnt; i++ {
		go func() {
			for d := range msgs {
				handlerRMQMsgWithDeliveryHandler(autoAck, d, queueName, deliveryHandler)
			}
		}()
	}
}

func (rmq *RMQ) Consume(queueName, bindKeys string, autoAck bool, goroutineCnt int, deliveryHandler func(d *amqp.Delivery) error) {
	rmq.ConsumeWithDelivery(queueName, bindKeys, autoAck, goroutineCnt, func(d *amqp.Delivery) (bool, error) {
		err := deliveryHandler(d)
		return false, err
	})
}
