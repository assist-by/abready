package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"

	lib "github.com/assist-by/autro-library"
)

var (
	redisClient       *redis.Client
	ctx               = context.Background()
	kafkaBroker       string
	registrationTopic string
)

func init() {
	// redis 초기화
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	redisClient = redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	_, err := redisClient.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	kafkaBroker = os.Getenv("KAFKA_BROKER")
	if kafkaBroker == "" {
		kafkaBroker = "kafka:9092"
	}
	registrationTopic = os.Getenv("REGISTRATION_TOPIC")
	if registrationTopic == "" {
		registrationTopic = "service-registration"
	}

}

// kafka consumer가 메시지 받기 시작
func startKafkaConsumer() {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{kafkaBroker},
		Topic:       registrationTopic,
		MaxAttempts: 5,
	})

	defer reader.Close()

	registerService(reader)
}

// 유효성 검사 함수
func validateService(service *lib.Service) error {
	if service.Name == "" {
		return fmt.Errorf("service name cannot be empty")
	}
	if service.Address == "" {
		return fmt.Errorf("service Address cannot be empty")
	}

	return nil
}

// 서비스 등록 함수
func registerService(reader *kafka.Reader) {
	for {
		msg, err := reader.ReadMessage(context.Background())
		if err != nil {
			log.Printf("Error reading message: %v", err)
			continue
		}

		var service lib.Service
		err = json.Unmarshal(msg.Value, &service)
		if err != nil {
			log.Printf("Error unmarshaling service data: %v", err)
			continue
		}

		service.LastHeartbeat = time.Now()

		serviceJSON, err := json.Marshal(service)
		if err != nil {
			log.Printf("Error marshaling service: %v", err)
			continue
		}

		err = redisClient.Set(ctx, service.Name, serviceJSON, 0).Err()
		if err != nil {
			log.Printf("Error registering service: %v", err)
			continue
		}

		err = redisClient.SAdd(ctx, "all:services", service.Name).Err()
		if err != nil {
			log.Printf("Error adding service to set: %v", err)
			continue
		}

		log.Printf("Service registered: %s", service.Name)
	}
}

// 서비스 업데이트 함수
// 서비스 업데이트 함수는 지금은 이름을 변경하지 않는다는 전제가 있다.
func updateService(c *gin.Context) {
	name := c.Param("name")

	var service lib.Service
	if err := c.ShouldBindJSON(&service); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := validateService(&service); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := redisClient.Watch(ctx, func(tx *redis.Tx) error {
		exists, err := redisClient.Exists(ctx, name).Result()
		if err != nil {
			return err
		}
		if exists == 0 {
			return fmt.Errorf("service not found")
		}
		service.LastHeartbeat = time.Now()

		serviceJSON, err := json.Marshal(service)
		if err != nil {
			return err
		}

		_, err = tx.TxPipelined(ctx, func(p redis.Pipeliner) error {
			return p.Set(ctx, name, serviceJSON, 0).Err()
		})
		return err
	}, name)

	if err != nil {
		if err == redis.TxFailedErr {
			c.JSON(http.StatusConflict, gin.H{"error": "Transaction failed, please try again"})
		} else if err.Error() == "service not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Service not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update service"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Service updated sucessfully."})
}

// 서비스 제거 함수
func deleteService(c *gin.Context) {
	name := c.Param("name")

	exists, err := redisClient.Exists(ctx, name).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check service existence"})
		return
	}
	if exists == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Service not found"})
		return
	}

	err = redisClient.Del(ctx, name).Err()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete service"})
		return
	}

	err = redisClient.SRem(ctx, "all:services", name).Err()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove service from set"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Service deleted successfully"})
}

// 서비스 리스트 반환 함수
func listServices(c *gin.Context) {
	serviceNames, err := redisClient.SMembers(ctx, "all:services").Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list services"})
		return
	}

	services := make(map[string]lib.Service)
	for _, name := range serviceNames {
		serviceJSON, err := redisClient.Get(ctx, name).Result()
		if err != nil {
			log.Printf("Failed to get service %s: %v", name, err)
			continue
		}

		var service lib.Service
		err = json.Unmarshal([]byte(serviceJSON), &service)
		if err != nil {
			log.Printf("Failed to unmarshal service %s: %v", name, err)
			continue
		}
		services[name] = service
	}

	c.JSON(http.StatusOK, services)
}

// 서비스 반환 함수
func getService(c *gin.Context) {
	name := c.Param("name")

	serviceJSON, err := redisClient.Get(ctx, name).Result()
	if err == redis.Nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Service not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get service"})
		return
	}

	var service lib.Service
	err = json.Unmarshal([]byte(serviceJSON), &service)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unmarshal service"})
		return
	}

	c.JSON(http.StatusOK, service)
}

// 헬스체크 함수
func healthCheck(c *gin.Context) {
	name := c.Param("name")

	err := redisClient.Watch(ctx, func(tx *redis.Tx) error {
		exists, err := tx.Exists(ctx, name).Result()
		if err != nil {
			return err
		}
		if exists == 0 {
			return fmt.Errorf("service not found")
		}

		serviceJSON, err := tx.Get(ctx, name).Result()
		if err != nil {
			return err
		}

		var service lib.Service
		if err := json.Unmarshal([]byte(serviceJSON), &service); err != nil {
			return err
		}

		service.LastHeartbeat = time.Now()
		updatedServiceJSON, err := json.Marshal(service)
		if err != nil {
			return err
		}

		_, err = tx.TxPipelined(ctx, func(p redis.Pipeliner) error {
			return p.Set(ctx, name, updatedServiceJSON, 0).Err()
		})
		return err
	}, name)

	if err != nil {
		if err == redis.TxFailedErr {
			c.JSON(http.StatusConflict, gin.H{"error": "Transaction failed, please try again"})
		} else if err.Error() == "service not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Service not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update service health"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Health check successful"})
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "up"})
}

func main() {
	go startKafkaConsumer()

	router := gin.Default()

	router.GET("/services", listServices)
	router.GET("/services/:name", getService)
	router.PUT("/services/:name", updateService)
	router.DELETE("/services/:name", deleteService)
	router.POST("/health/:name", healthCheck)
	router.GET("/health", healthHandler)
	router.HEAD("/health", healthHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8500"
	}

	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}

}
