# poc-go-rabbitmq
This is a simple example of how to use RabbitMQ with Go.

```mermaid
sequenceDiagram
    participant Client
    participant Producer
    participant PostgreSQL
    participant RabbitMQ
    participant Consumer
    participant Callback

    Client->>Producer: Request to start a job
    activate Producer
    Producer->>PostgreSQL: Store job metadata
    activate PostgreSQL
    note right of PostgreSQL: Job metadata is saved
    deactivate PostgreSQL
    Producer->>RabbitMQ: Publish job with metadata
    activate RabbitMQ
    note right of RabbitMQ: Job is stored in a queue
    deactivate RabbitMQ
    Producer->>Client: Acknowledgment (Job ID or status)
    deactivate Producer

    RabbitMQ->>Consumer: Deliver job with metadata
    activate Consumer
    note right of Consumer: Job is being processed
    Consumer->>Consumer: Process job
    alt Job succeeds
        Consumer->>PostgreSQL: Update job metadata
        activate PostgreSQL
        note right of PostgreSQL: Job metadata is updated
        deactivate PostgreSQL
        Consumer->>Callback: Send result to callback URL
        activate Callback
        note right of Callback: Callback processes result (maybe the same as Client)
        deactivate Callback
        Consumer->>RabbitMQ: Acknowledge job completion
        activate RabbitMQ
        note right of RabbitMQ: Job is removed from the queue
        deactivate RabbitMQ
    else Job fails
        Consumer->>PostgreSQL: Increment job attempts
        activate PostgreSQL
        note right of PostgreSQL: Job attempts updated
        deactivate PostgreSQL
        alt Retry limit not reached
            Consumer->>RabbitMQ: Requeue the job
            activate RabbitMQ
            note right of RabbitMQ: Job is requeued for another try
            deactivate RabbitMQ
        else Retry limit reached
            Consumer->>PostgreSQL: Mark job as failed
            activate PostgreSQL
            note right of PostgreSQL: Job marked as failed
            deactivate PostgreSQL
            Consumer->>RabbitMQ: Acknowledge job completion
            activate RabbitMQ
            note right of RabbitMQ: Job is removed from the queue
            deactivate RabbitMQ
        end
    end
    deactivate Consumer

```
> PostgreSQL is used to store the job metadata. (but not implemented yet)

## Prerequisites
- Docker
- Go v1.20

## Getting Started
1. Clone this repository
2. Run the following command to start the all services
```bash
docker compose up -d --build --scale go-consumer=3
```
3. Call the following endpoint to send a message to the queue
```bash
# Start a long-running task
curl --location --request POST 'http://localhost:3099/start-task'

# Get the status of the task (you can use the id from the previous command)
curl --location 'http://localhost:3099/jobs/:id'

# Retry the task (you can use the id from the previous command)
curl --location --request POST 'http://localhost:3099/jobs/:id/retry'
```
4. See the message in the consumer logs
```bash
docker compose logs -f go-consumer
```
5. enjoy!