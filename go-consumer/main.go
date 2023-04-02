package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/streadway/amqp"
)

type Job struct {
	ID       string
	Status   string
	Callback string
}

func main() {
	onClose := make(chan *amqp.Error)

	go func() {
		for {
			conn, ch, _ := setupRabbitMQ("amqp://myuser:mypassword@rabbitmq:5672/", onClose)
			consumeTasks(ch, onClose)
			log.Println("RabbitMQ channel closed. Reconnecting...")
			conn.Close()
		}
	}()

	// Keep the consumer running indefinitely
	forever := make(chan bool)
	<-forever
}

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func connectRabbitMQ(url string) (*amqp.Connection, error) {
	var conn *amqp.Connection

	// Configure exponential backoff
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 0 // Retry indefinitely

	err := backoff.RetryNotify(func() error {
		var err error
		conn, err = amqp.Dial(url)
		return err
	}, bo, func(err error, d time.Duration) {
		log.Printf("Failed to connect to RabbitMQ. Retrying in %s...", d)
	})

	if err != nil {
		return nil, err
	}

	return conn, nil
}

func reconnectRabbitMQ(url string, onClose chan *amqp.Error) (*amqp.Connection, *amqp.Channel, error) {
	for {
		conn, err := connectRabbitMQ(url)
		if err == nil {
			ch, err := conn.Channel()
			if err == nil {
				ch.NotifyClose(onClose)
				return conn, ch, nil
			}
		}

		log.Println("Failed to connect to RabbitMQ. Retrying in 5 seconds...")
		time.Sleep(5 * time.Second)
	}
}

func setupRabbitMQ(url string, onClose chan *amqp.Error) (*amqp.Connection, *amqp.Channel, amqp.Queue) {
	conn, ch, err := reconnectRabbitMQ(url, onClose)
	failOnError(err, "Failed to set up RabbitMQ")

	q, err := ch.QueueDeclare(
		"tasks",
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // args
	)
	failOnError(err, "Failed to declare a queue")

	return conn, ch, q
}

func consumeTasks(ch *amqp.Channel, onClose chan *amqp.Error) {
	q, err := ch.QueueDeclare(
		"tasks",
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // args
	)
	failOnError(err, "Failed to declare a queue")

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	failOnError(err, "Failed to register a consumer")

	go func() {
		for d := range msgs {
			var job Job
			err := json.Unmarshal(d.Body, &job)
			if err != nil {
				log.Printf("Failed to unmarshal task message: %v", err)
				d.Nack(false, true) // Send a negative acknowledgement and requeue the message
				continue
			}
			log.Printf("Received a task: %s", job.ID)

			// Simulate long-running task
			time.Sleep(60 * time.Second)

			// Send a status update to the callback URL
			job.Status = "done"
			sendJobStatusUpdate(job)

			log.Printf("Finished task: %s", job.ID)
			d.Ack(false) // Acknowledge the message
		}
	}()

	<-onClose
}

func sendJobStatusUpdate(job Job) {
	client := &http.Client{}
	data := url.Values{}
	data.Set("job_id", job.ID)
	data.Set("status", job.Status)

	req, err := http.NewRequest("POST", job.Callback, strings.NewReader(data.Encode()))
	if err != nil {
		log.Printf("Failed to create request for job status update: %v", err)
		return
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to send job status update: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Failed to update job status, received status code: %d", resp.StatusCode)
	}
}
