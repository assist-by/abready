package main

import (
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	lib "github.com/with-autro/autro-library"
)

var (
	services   = make(map[string]*lib.Service)
	servicesMu sync.RWMutex
)

// 서비스 등록 함수
func registerService(c *gin.Context) {
	var service lib.Service
	if err := c.ShouldBindJSON(&service); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	servicesMu.Lock()
	services[service.Name] = &service
	servicesMu.Unlock()

	c.JSON(http.StatusOK, gin.H{"message": "Service registered successfully"})
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

func main() {
	router := gin.Default()

	router.POST("/register", registerService)
	router.GET("/services", listServices)
	router.GET("/services/:name", getService)

	if err := router.Run(":8500"); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
