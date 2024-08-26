package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	lib "github.com/with-autro/autro-library"
)

var (
	services   = make(map[string]*lib.Service)
	servicesMu sync.RWMutex
)

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
func registerService(c *gin.Context) {
	var service lib.Service
	if err := c.ShouldBindJSON(&service); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := validateService(&service); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	service.LastHeartbeat = time.Now()

	servicesMu.Lock()
	services[service.Name] = &service
	servicesMu.Unlock()

	c.JSON(http.StatusOK, gin.H{"message": "Service registered successfully"})
}

// 서비스 업데이트 함수
func updateService(c *gin.Context) {
	name := c.Param("name")

	var service lib.Service
	if err := c.ShouldBindJSON(&service); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := validateService(&service); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}

	servicesMu.Lock()
	if _, exists := services[name]; !exists {
		servicesMu.Unlock()
		c.JSON(http.StatusNotFound, gin.H{"error": "Service not found."})
		return
	}

	service.LastHeartbeat = time.Now()
	services[name] = &service
	servicesMu.Unlock()

	c.JSON(http.StatusOK, gin.H{"message": "Service updated sucessfully."})
}

// 서비스 제거 함수
func deleteService(c *gin.Context) {
	name := c.Param("name")

	servicesMu.Lock()
	if _, exists := services[name]; !exists {
		servicesMu.Unlock()
		c.JSON(http.StatusNotFound, gin.H{"error": "Service not found"})
		return
	}

	delete(services, name)
	servicesMu.Unlock()
	c.JSON(http.StatusOK, gin.H{"message": "Service deleted successfully"})
}

// 서비스 리스트 반환 함수
func listServices(c *gin.Context) {
	servicesMu.RLock()
	defer servicesMu.RUnlock()

	c.JSON(http.StatusOK, services)
}

// 서비스 반환 함수
func getService(c *gin.Context) {
	name := c.Param("name")

	servicesMu.RLock()
	service, exists := services[name]
	servicesMu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Service not found"})
		return
	}

	c.JSON(http.StatusOK, service)
}

// 헬스체크 함수
func healthCheck(c *gin.Context) {
	name := c.Param("name")
	servicesMu.Lock()
	defer servicesMu.Unlock()

	service, exists := services[name]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Service not found"})
		return
	}

	service.LastHeartbeat = time.Now()
	c.JSON(http.StatusOK, gin.H{"message": "Health check successful"})
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "up"})
}

func main() {
	router := gin.Default()

	router.GET("/services", listServices)
	router.POST("/register", registerService)
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
