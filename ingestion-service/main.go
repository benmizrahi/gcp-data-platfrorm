package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"cloud.google.com/go/pubsub"
	eventsv1 "github.com/gcp-data-platform/schema-management/gen/go/platform/events/v1"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// PubSubPublisher manages connections and publishing of messages to GCP Pub/Sub.
type PubSubPublisher struct {
	client *pubsub.Client
	topics map[string]*pubsub.Topic
}

// NewPubSubPublisher initializes a new Pub/Sub client and verifies topics exists.
func NewPubSubPublisher(ctx context.Context, projectID string, topicNames map[string]string) (*PubSubPublisher, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create pubsub client: %w", err)
	}

	topics := make(map[string]*pubsub.Topic)
	for key, topicName := range topicNames {
		topic := client.Topic(topicName)
		exists, err := topic.Exists(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to check if topic %s exists: %w", topicName, err)
		}
		if !exists {
			return nil, fmt.Errorf("topic %s does not exist in project %s", topicName, projectID)
		}
		topics[key] = topic
	}

	return &PubSubPublisher{
		client: client,
		topics: topics,
	}, nil
}

// PublishEvent serializes a protobuf message to JSON and publishes it to the designated topic.
func (p *PubSubPublisher) PublishEvent(ctx context.Context, key string, msg proto.Message) error {
	if p == nil || p.client == nil {
		log.Printf("[LOCAL LOG ONLY] Pub/Sub client not initialized. Ingested Event Payload: %s\n", protojson.Format(msg))
		return nil
	}

	topic, ok := p.topics[key]
	if !ok {
		return fmt.Errorf("no topic configured for key: %s", key)
	}

	// Serialize to JSON using protojson to strictly adhere to Pub/Sub schema requirements
	marshalOpts := protojson.MarshalOptions{
		EmitUnpopulated: true,
		UseProtoNames:   true,
	}
	jsonBytes, err := marshalOpts.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to serialize event to JSON: %w", err)
	}

	res := topic.Publish(ctx, &pubsub.Message{
		Data: jsonBytes,
	})

	id, err := res.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to publish message to Pub/Sub: %w", err)
	}

	log.Printf("Successfully published event to Pub/Sub topic %s [MsgID: %s]\n", topic.ID(), id)
	return nil
}

// ingestionServer implements the gRPC IngestionServiceServer interface.
type ingestionServer struct {
	eventsv1.UnimplementedIngestionServiceServer
	publisher *PubSubPublisher
}

// IngestEvent is the single gRPC endpoint that accepts any telemetry event and publishes it to Pub/Sub.
func (s *ingestionServer) IngestEvent(ctx context.Context, req *eventsv1.IngestEventRequest) (*eventsv1.IngestEventResponse, error) {
	if req.Payload == nil {
		return nil, status.Error(codes.InvalidArgument, "missing event payload")
	}

	var eventID string
	var topicKey string
	var eventMsg proto.Message

	switch p := req.Payload.(type) {
	case *eventsv1.IngestEventRequest_Login:
		event := p.Login
		if event.EventId == "" {
			event.EventId = uuid.New().String()
		}
		if event.Timestamp == nil {
			now := time.Now()
			event.Timestamp = &eventsv1.LoginEvent_Timestamp{
				Seconds: now.Unix(),
				Nanos:   int32(now.Nanosecond()),
			}
		}
		if event.EventName == "" {
			event.EventName = "user.login"
		}
		if event.Source == "" {
			event.Source = "grpc-ingestion-service"
		}
		eventID = event.EventId
		topicKey = "login"
		eventMsg = event

		log.Printf("--> Received Login Event via gRPC [ID: %s, Source: %s, User: %s]\n",
			event.EventId, event.Source, event.UserId)

	case *eventsv1.IngestEventRequest_Level:
		event := p.Level
		if event.EventId == "" {
			event.EventId = uuid.New().String()
		}
		if event.Timestamp == nil {
			now := time.Now()
			event.Timestamp = &eventsv1.LevelEvent_Timestamp{
				Seconds: now.Unix(),
				Nanos:   int32(now.Nanosecond()),
			}
		}
		if event.EventName == "" {
			event.EventName = "level.progression"
		}
		if event.Source == "" {
			event.Source = "grpc-ingestion-service"
		}
		eventID = event.EventId
		topicKey = "level"
		eventMsg = event

		log.Printf("--> Received Level Event via gRPC [ID: %s, Source: %s, User: %s]\n",
			event.EventId, event.Source, event.UserId)

	case *eventsv1.IngestEventRequest_Transaction:
		event := p.Transaction
		if event.EventId == "" {
			event.EventId = uuid.New().String()
		}
		if event.Timestamp == nil {
			now := time.Now()
			event.Timestamp = &eventsv1.TransactionEvent_Timestamp{
				Seconds: now.Unix(),
				Nanos:   int32(now.Nanosecond()),
			}
		}
		if event.EventName == "" {
			event.EventName = "commerce.purchase"
		}
		if event.Source == "" {
			event.Source = "grpc-ingestion-service"
		}
		eventID = event.EventId
		topicKey = "transaction"
		eventMsg = event

		log.Printf("--> Received Transaction Event via gRPC [ID: %s, Source: %s, User: %s]\n",
			event.EventId, event.Source, event.UserId)

	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported event type payload")
	}

	// Publish to Pub/Sub
	if err := s.publisher.PublishEvent(ctx, topicKey, eventMsg); err != nil {
		log.Printf("Failed to publish event to Pub/Sub: %v\n", err)
		return nil, status.Errorf(codes.Internal, "failed to publish event: %v", err)
	}

	return &eventsv1.IngestEventResponse{
		EventId: eventID,
		Status:  "SUCCESS",
	}, nil
}

// grpcAuthInterceptor verifies API token credentials on incoming gRPC calls.
func grpcAuthInterceptor(apiToken string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing request metadata")
		}

		// Attempt to extract token from 'authorization' metadata
		tokens := md.Get("authorization")
		if len(tokens) == 0 {
			// Fall back to custom 'x-api-token' metadata
			tokens = md.Get("x-api-token")
		}

		if len(tokens) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing ingestion API token")
		}

		token := tokens[0]
		const bearerPrefix = "Bearer "
		if len(token) > len(bearerPrefix) && token[:len(bearerPrefix)] == bearerPrefix {
			token = token[len(bearerPrefix):]
		}

		if token != apiToken {
			return nil, status.Error(codes.Unauthenticated, "invalid ingestion API token")
		}

		return handler(ctx, req)
	}
}

// fiberAuthMiddleware verifies API token credentials on incoming HTTP JSON calls.
func fiberAuthMiddleware(apiToken string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Allow anonymous health check calls
		if c.Path() == "/healthz" {
			return c.Next()
		}

		// Extract token from standard Authorization header
		token := c.Get("Authorization")
		if token == "" {
			// Fall back to custom X-API-Token header
			token = c.Get("X-API-Token")
		}

		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing API token",
			})
		}

		// Remove Bearer scheme prefix if present
		const bearerPrefix = "Bearer "
		if len(token) > len(bearerPrefix) && token[:len(bearerPrefix)] == bearerPrefix {
			token = token[len(bearerPrefix):]
		}

		if token != apiToken {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid API token",
			})
		}

		return c.Next()
	}
}

func main() {
	// Load environment variables from .env file if present
	if err := godotenv.Load(); err != nil {
		log.Println("INFO: No .env file found, falling back to system environment variables")
	}

	// 1. Load Environment Configurations
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		projectID = "catchme-poc"
	}

	loginTopic := os.Getenv("PUBSUB_TOPIC_LOGIN")
	if loginTopic == "" {
		loginTopic = "dev-platform-login"
	}

	levelTopic := os.Getenv("PUBSUB_TOPIC_LEVEL")
	if levelTopic == "" {
		levelTopic = "dev-platform-level"
	}

	txTopic := os.Getenv("PUBSUB_TOPIC_TRANSACTION")
	if txTopic == "" {
		txTopic = "dev-platform-transaction"
	}

	topicNames := map[string]string{
		"login":       loginTopic,
		"level":       levelTopic,
		"transaction": txTopic,
	}

	// Load secure API token from env
	apiToken := os.Getenv("INGESTION_API_TOKEN")
	if apiToken == "" {
		apiToken = "dev-secure-token-12345" // Local development default
		log.Printf("WARNING: INGESTION_API_TOKEN env variable is not set. Using default dev token: %s\n", apiToken)
	}

	// 2. Initialize Pub/Sub Publisher with elegant fallback for local development
	ctx := context.Background()
	publisher, err := NewPubSubPublisher(ctx, projectID, topicNames)
	if err != nil {
		log.Printf("WARNING: Google Cloud Pub/Sub client could not be initialized: %v.\n", err)
		log.Println("--> Falling back to local development mode (events will be logged locally without publishing).")
	} else {
		log.Printf("Successfully connected to GCP Pub/Sub in project %s.\n", projectID)
	}

	// 3. Start protected gRPC Ingestion Server in the background
	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50051"
	}
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		log.Fatalf("Failed to bind TCP listener on gRPC port %s: %v", grpcPort, err)
	}

	// Create server with Unary Auth Interceptor
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpcAuthInterceptor(apiToken)),
	)
	eventsv1.RegisterIngestionServiceServer(grpcServer, &ingestionServer{publisher: publisher})

	go func() {
		log.Printf("Starting Ingestion gRPC Server on port %s...\n", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to start gRPC server: %v", err)
		}
	}()

	// 4. Initialize and Run Fiber HTTP Application
	app := fiber.New(fiber.Config{
		AppName: "GCP Data Platform HTTP Ingestion Service v2.0",
	})

	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} ${latency}\n",
	}))

	// Apply Fiber API Token Middleware
	app.Use(fiberAuthMiddleware(apiToken))

	// Health Check (Unprotected route bypassed by middleware)
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status":  "healthy",
			"service": "ingestion-service",
			"grpc":    "active",
			"pubsub":  publisher != nil,
		})
	})

	// Individual Route Endpoint forwards (REST style)
	app.Post("/api/v1/events/login", func(c *fiber.Ctx) error {
		var event eventsv1.LoginEvent
		if err := protojson.Unmarshal(c.Body(), &event); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid LoginEvent schema", "details": err.Error()})
		}
		if event.EventId == "" {
			event.EventId = uuid.New().String()
		}
		if event.Timestamp == nil {
			now := time.Now()
			event.Timestamp = &eventsv1.LoginEvent_Timestamp{Seconds: now.Unix(), Nanos: int32(now.Nanosecond())}
		}
		if event.EventName == "" {
			event.EventName = "user.login"
		}
		if event.Source == "" {
			event.Source = "http-ingestion-service"
		}
		log.Printf("--> Received Login Event over HTTP [ID: %s, Name: %s, Source: %s, User: %s]\n",
			event.EventId, event.EventName, event.Source, event.UserId)
		if err := publisher.PublishEvent(c.Context(), "login", &event); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"event_id": event.EventId, "status": "SUCCESS"})
	})

	app.Post("/api/v1/events/level", func(c *fiber.Ctx) error {
		var event eventsv1.LevelEvent
		if err := protojson.Unmarshal(c.Body(), &event); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid LevelEvent schema", "details": err.Error()})
		}
		if event.EventId == "" {
			event.EventId = uuid.New().String()
		}
		if event.Timestamp == nil {
			now := time.Now()
			event.Timestamp = &eventsv1.LevelEvent_Timestamp{Seconds: now.Unix(), Nanos: int32(now.Nanosecond())}
		}
		if event.EventName == "" {
			event.EventName = "level.progression"
		}
		if event.Source == "" {
			event.Source = "http-ingestion-service"
		}
		log.Printf("--> Received Level Event over HTTP [ID: %s, Name: %s, Source: %s, User: %s]\n",
			event.EventId, event.EventName, event.Source, event.UserId)
		if err := publisher.PublishEvent(c.Context(), "level", &event); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"event_id": event.EventId, "status": "SUCCESS"})
	})

	app.Post("/api/v1/events/transaction", func(c *fiber.Ctx) error {
		var event eventsv1.TransactionEvent
		if err := protojson.Unmarshal(c.Body(), &event); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid TransactionEvent schema", "details": err.Error()})
		}
		if event.EventId == "" {
			event.EventId = uuid.New().String()
		}
		if event.Timestamp == nil {
			now := time.Now()
			event.Timestamp = &eventsv1.TransactionEvent_Timestamp{Seconds: now.Unix(), Nanos: int32(now.Nanosecond())}
		}
		if event.EventName == "" {
			event.EventName = "commerce.purchase"
		}
		if event.Source == "" {
			event.Source = "http-ingestion-service"
		}
		log.Printf("--> Received Transaction Event over HTTP [ID: %s, Name: %s, Source: %s, User: %s]\n",
			event.EventId, event.EventName, event.Source, event.UserId)
		if err := publisher.PublishEvent(c.Context(), "transaction", &event); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"event_id": event.EventId, "status": "SUCCESS"})
	})

	// Unified HTTP Route (Single endpoint accepting ANY event payload dynamically)
	app.Post("/api/v1/events", func(c *fiber.Ctx) error {
		body := c.Body()

		var typeDetector struct {
			EventName string `json:"event_name"`
		}
		if err := json.Unmarshal(body, &typeDetector); err != nil || typeDetector.EventName == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "missing or invalid event_name in payload",
			})
		}

		ctx := c.Context()
		switch typeDetector.EventName {
		case "user.login":
			var event eventsv1.LoginEvent
			if err := protojson.Unmarshal(body, &event); err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid LoginEvent schema", "details": err.Error()})
			}
			if event.EventId == "" {
				event.EventId = uuid.New().String()
			}
			if event.Timestamp == nil {
				now := time.Now()
				event.Timestamp = &eventsv1.LoginEvent_Timestamp{Seconds: now.Unix(), Nanos: int32(now.Nanosecond())}
			}
			if event.EventName == "" {
				event.EventName = "user.login"
			}
			if event.Source == "" {
				event.Source = "http-ingestion-service"
			}
			log.Printf("--> Received Login Event over HTTP [ID: %s, Name: %s, Source: %s, User: %s]\n",
				event.EventId, event.EventName, event.Source, event.UserId)
			if err := publisher.PublishEvent(ctx, "login", &event); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"event_id": event.EventId, "status": "SUCCESS"})

		case "level.progression":
			var event eventsv1.LevelEvent
			if err := protojson.Unmarshal(body, &event); err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid LevelEvent schema", "details": err.Error()})
			}
			if event.EventId == "" {
				event.EventId = uuid.New().String()
			}
			if event.Timestamp == nil {
				now := time.Now()
				event.Timestamp = &eventsv1.LevelEvent_Timestamp{Seconds: now.Unix(), Nanos: int32(now.Nanosecond())}
			}
			if event.EventName == "" {
				event.EventName = "level.progression"
			}
			if event.Source == "" {
				event.Source = "http-ingestion-service"
			}
			log.Printf("--> Received Level Event over HTTP [ID: %s, Name: %s, Source: %s, User: %s]\n",
				event.EventId, event.EventName, event.Source, event.UserId)
			if err := publisher.PublishEvent(ctx, "level", &event); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"event_id": event.EventId, "status": "SUCCESS"})

		case "commerce.purchase":
			var event eventsv1.TransactionEvent
			if err := protojson.Unmarshal(body, &event); err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid TransactionEvent schema", "details": err.Error()})
			}
			if event.EventId == "" {
				event.EventId = uuid.New().String()
			}
			if event.Timestamp == nil {
				now := time.Now()
				event.Timestamp = &eventsv1.TransactionEvent_Timestamp{Seconds: now.Unix(), Nanos: int32(now.Nanosecond())}
			}
			if event.EventName == "" {
				event.EventName = "commerce.purchase"
			}
			if event.Source == "" {
				event.Source = "http-ingestion-service"
			}
			log.Printf("--> Received Transaction Event over HTTP [ID: %s, Name: %s, Source: %s, User: %s]\n",
				event.EventId, event.EventName, event.Source, event.UserId)
			if err := publisher.PublishEvent(ctx, "transaction", &event); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"event_id": event.EventId, "status": "SUCCESS"})

		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("unsupported event_name: %s", typeDetector.EventName),
			})
		}
	})

	// Start Fiber Server
	httpPort := os.Getenv("PORT")
	if httpPort == "" {
		httpPort = "8080"
	}
	log.Printf("Starting Ingestion HTTP Server on port %s...\n", httpPort)
	if err := app.Listen(fmt.Sprintf(":%s", httpPort)); err != nil {
		log.Fatalf("Failed to start Fiber server: %v", err)
	}
}
