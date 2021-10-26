package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"sync"

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
	var err error

	db, err := setupDB()
	if err != nil {
		log.Fatalf("error setting up db: %v", err)
	}

	indexTemplate := template.Must(template.ParseFiles("./static/html/index.html"))
	notFoundTemplate := template.Must(template.ParseFiles("./static/html/404.html"))

	autocomplete, err := discover.NewAutocomplete()
	if err != nil {
		log.Fatalf("error creating discover client: %v", err)
	}

	var workers sync.WaitGroup

	defer func() {
		log.Printf("waiting for all sync workers to finish their work")

		workers.Wait()
	}()

	adminPort := flag.Int("admin-port", defaultAdminPort, "admin port used for configuration and monitoring")
	adminServer := admin.NewServer(&routes, &workers, indexTemplate, notFoundTemplate, autocomplete, db)
	go adminServer.ListenAndServe(*adminPort)

	port := flag.Int("port", defaultPort, "main port used for access")
	routerServer := router.NewServer(&routes)
	routerServer.ListenAndServe(*port)
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
