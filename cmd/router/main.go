package main

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sync"
	"syscall"

	"github.com/iskorotkov/router/internal/admin"
	"github.com/iskorotkov/router/internal/discover"
	"github.com/iskorotkov/router/internal/models"
	"github.com/iskorotkov/router/internal/router"
	"github.com/iskorotkov/router/internal/routing"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	dataFolder = "data"
	dbName     = "db.sqlite"

	dataFolderPermissions = os.FileMode(0777) //nolint:gofumpt

	defaultPort      = 8080
	defaultAdminPort = 7676
)

//nolint:gochecknoglobals
var routes routing.Cache

func main() {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("panic occurred: %v", p)
			debug.PrintStack()
		}
	}()

	var err error

	db, err := setupDB()
	if err != nil {
		log.Printf("error setting up db: %v", err)

		return
	}

	indexTemplate := template.Must(template.ParseFiles("./static/html/index.html"))
	notFoundTemplate := template.Must(template.ParseFiles("./static/html/404.html"))

	autocomplete, err := discover.NewAutocomplete()
	if err != nil {
		log.Printf("error creating discover client: %v", err)

		return
	}

	var workers sync.WaitGroup

	defer func() {
		log.Printf("waiting for all sync workers to finish their work")

		workers.Wait()

		log.Printf("all workers completed")
	}()

	adminPort := flag.Int("admin-port", defaultAdminPort, "admin port used for configuration and monitoring")
	adminServer := admin.NewServer(&routes, &workers, indexTemplate, notFoundTemplate, autocomplete, db)

	port := flag.Int("port", defaultPort, "main port used for access")
	routerServer := router.NewServer(&routes)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go adminServer.ListenAndServe(ctx, *adminPort)
	go routerServer.ListenAndServe(ctx, *port)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)

	log.Printf("shutdown signal received: %v", <-ch)
}

func setupDB() (*gorm.DB, error) {
	if err := os.MkdirAll(dataFolder, dataFolderPermissions); err != nil {
		return nil, fmt.Errorf("error creating data folder: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(filepath.Join(dataFolder, dbName)))
	if err != nil {
		return nil, fmt.Errorf("error opening database: %w", err)
	}

	if err := db.AutoMigrate(
		&models.Route{}, //nolint:exhaustivestruct
	); err != nil {
		return nil, fmt.Errorf("error running migrations: %w", err)
	}

	populateRoutes(db)

	return db, nil
}

func populateRoutes(db *gorm.DB) {
	var storedRoutes []models.Route

	if err := db.Order("id DESC").Find(&storedRoutes).Error; err != nil {
		log.Fatalf("error reading stored routes from db: %v", err)
	}

	routes = routing.New()

	for _, route := range storedRoutes {
		routes.Set(route.From, routing.RouteInfo{
			To:   route.To,
			Type: route.Type,
		})
	}
}
