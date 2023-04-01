# poc-go-rabbitmq
This is a simple example of how to use RabbitMQ with Go.

## Prerequisites
- Docker
- Go v1.20

## Getting Started
1. Clone this repository
2. Run the following command to start the all services
```bash
docker-compose up -d --build
```
3. Call the following endpoint to send a message to the queue
```bash
curl -X POST http://localhost:8080/send
```
4. See the message in the consumer logs
```bash
docker-compose logs -f go-consumer
```
5. enjoy!