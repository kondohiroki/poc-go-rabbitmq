package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/streadway/amqp"
)

type Job struct {
	ID       string
	Status   string
	Callback string
}

func main() {
	onClose := make(chan *amqp.Error)
	var conn *amqp.Connection
	var ch *amqp.Channel
	var newCh *amqp.Channel
	var q amqp.Queue

	go func() {
		for {
			conn, newCh, q = setupRabbitMQ("amqp://myuser:mypassword@rabbitmq:5672/", onClose)
			ch = newCh
			<-onClose
			log.Println("RabbitMQ channel closed. Reconnecting...")
			conn.Close()
		}
	}()

	// Wait for the first setup to complete
	time.Sleep(5 * time.Second)

	app := fiber.New()

	var jobs = make(map[string]*Job)

	app.Post("/start-task", func(c *fiber.Ctx) error {
		jobID := uuid.New().String()
		job := &Job{
			ID:       jobID,
			Status:   "queued",
			Callback: "https://4d368900-e5d5-45ab-a212-ce230bf44ad2.mock.pstmn.io/callback",
		}
		jobs[jobID] = job

		taskMessage, err := json.Marshal(job)
		failOnError(err, "Failed to marshal task message")

		err = ch.Publish(
			"",     // exchange
			q.Name, // routing key
			false,  // mandatory
			false,  // immediate
			amqp.Publishing{
				ContentType:  "application/json",
				Body:         taskMessage,
				DeliveryMode: amqp.Persistent, // Set the DeliveryMode to Persistent
			})
		failOnError(err, "Failed to publish a task")

		log.Printf("Sent a task: %s", job.ID)
		return c.JSON(fiber.Map{"job_id": job.ID})
	})

	app.Get("/jobs/:id", func(c *fiber.Ctx) error {
		jobID := c.Params("id")
		job, ok := jobs[jobID]
		if !ok {
			return c.Status(fiber.StatusNotFound).SendString("Job not found")
		}
		return c.JSON(job)
	})

	app.Post("/jobs/:id/retry", func(c *fiber.Ctx) error {
		jobID := c.Params("id")
		job, ok := jobs[jobID]
		if !ok {
			return c.Status(fiber.StatusNotFound).SendString("Job not found")
		}
		if job.Status != "done" && job.Status != "failed" {
			return c.Status(fiber.StatusBadRequest).SendString("Job cannot be retried")
		}

		job.Status = "queued"
		taskMessage, err := json.Marshal(job)
		failOnError(err, "Failed to marshal task message")

		err = ch.Publish(
			"",     // exchange
			q.Name, // routing key
			false,  // mandatory
			false,  // immediate
			amqp.Publishing{
				ContentType:  "application/json",
				Body:         taskMessage,
				DeliveryMode: amqp.Persistent, // Set the DeliveryMode to Persistent
			})
		failOnError(err, "Failed to publish a task")

		log.Printf("Retried task: %s", job.ID)
		return c.SendStatus(fiber.StatusAccepted)
	})

	app.Listen(":3000")
}

func connectRabbitMQ(url string) (*amqp.Connection, error) {
	var conn *amqp.Connection

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

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}
