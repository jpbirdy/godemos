package rmq

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"
)

const (
	queueName = "test.queue"
)

func TestConsumerMessage(t *testing.T) {
	rmq := New("amqp://admin:lyntime123456@rabbitmq.lyntime.com:5672/")

	rmq.RMQConsumeWithGoroutine(
		queueName,
		queueName,
		false,
		4, func(body []byte) (ack bool, err error) {
			log.Println(string(body))
			log.Println("xxxxxx")
			fmt.Println("xxxxx")
			time.Sleep(time.Second)
			return true, nil
		},
	)

	time.Sleep(20 * time.Second)
}

func TestPublishMessage(t *testing.T) {

	rmq := New("amqp://admin:lyntime123456@rabbitmq.lyntime.com:5672/")

	for i := 0; i < 10; i++ {
		testBody := struct {
			Index int64 `json:"index"`
			Time  int64 `json:"time"`
		}{
			Index: int64(i),
			Time:  time.Now().Unix(),
		}
		err := rmq.Publish(context.Background(), queueName, testBody)
		// err := agiRMQ.PublishWithUid(123, "tesla-test-be.delayMsg.postKey", testBody)
		fmt.Println(i, err)
		time.Sleep(1 * time.Second)
	}

	time.Sleep(10 * time.Second)
}
