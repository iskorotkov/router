package admin

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/iskorotkov/router/internal/discover"
	"github.com/iskorotkov/router/internal/models"
	"github.com/iskorotkov/router/internal/routing"
	"gorm.io/gorm"
)

var ErrValidation = fmt.Errorf("validation failed")

type Server struct {
	routes           *routing.Cache
	workers          *sync.WaitGroup
	indexTemplate    *template.Template
	notFoundTemplate *template.Template
	autocomplete     discover.Autocomplete
	db               *gorm.DB
}

func NewServer(
	routes *routing.Cache,
	workers *sync.WaitGroup,
	indexTemplate *template.Template,
	notFoundTemplate *template.Template,
	autocomplete discover.Autocomplete,
	db *gorm.DB,
) Server {
	return Server{
		routes:           routes,
		workers:          workers,
		indexTemplate:    indexTemplate,
		notFoundTemplate: notFoundTemplate,
		autocomplete:     autocomplete,
		db:               db,
	}
}

func (s Server) ListenAndServe(port int) {
	adminServer := http.NewServeMux()
	adminServer.HandleFunc("/api/v1/routes", func(rw http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.listRoutes(rw, r)
		case http.MethodPost:
			s.createRoute(rw, r)
		case http.MethodDelete:
			s.deleteRoute(rw, r)
		default:
			api404(rw, r)

			return
		}
	})
	adminServer.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir("./static/css"))))
	adminServer.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("./static/js"))))
	adminServer.HandleFunc("/", s.showDashboard)

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), adminServer); err != nil {
		log.Printf("error in admin server: %v", err)

		return
	}
}

func (s Server) listRoutes(rw http.ResponseWriter, _ *http.Request) {
	b, err := json.MarshalIndent(s.routes.GetAll(), "", "  ")
	if err != nil {
		log.Printf("error marshaling routes: %v", err)
		http.Error(rw, "", http.StatusInternalServerError)

		return
	}

	rw.Header().Add("Content-Type", http.DetectContentType(b))
	_, _ = fmt.Fprint(rw, string(b))
}

type createRouteDTO struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type models.RouteType
}

func (c *createRouteDTO) Validate() error {
	c.From = strings.TrimSpace(c.From)
	c.To = strings.TrimSpace(c.To)

	if c.From == "" || c.To == "" {
		return fmt.Errorf("one of the fields of %v is empty: %w", c, ErrValidation)
	}

	if c.Type != models.RouteTypeProxy && c.Type != models.RouteTypeRedirect {
		return fmt.Errorf("route type of %v is invalid: %w", c, ErrValidation)
	}

	return nil
}

func (s Server) createRoute(rw http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("error reading route from request body: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	var route createRouteDTO

	err = json.Unmarshal(b, &route)
	if err != nil {
		log.Printf("error unmarshaling route from request body: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	if err := route.Validate(); err != nil {
		log.Printf("error validating dto: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	s.routes.Set(route.From, routing.RouteInfo{
		To:   route.To,
		Type: route.Type,
	})

	s.workers.Add(1)

	go func() {
		defer s.workers.Done()

		if err := s.db.Save(&models.Route{
			Model: gorm.Model{}, //nolint:exhaustivestruct
			From:  route.From,
			To:    route.To,
			Type:  route.Type,
		}).Error; err != nil {
			log.Printf("error saving route to db: %v", err)

			return
		}

		log.Printf("route saved to db")
	}()
}

type deleteRouteDTO struct {
	From string `json:"from"`
}

func (d *deleteRouteDTO) Validate() error {
	d.From = strings.TrimSpace(d.From)

	if d.From == "" {
		return fmt.Errorf("one of the fields of %v is empty: %w", d, ErrValidation)
	}

	return nil
}

func (s Server) deleteRoute(rw http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("error reading route from request body: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	var route deleteRouteDTO

	err = json.Unmarshal(b, &route)
	if err != nil {
		log.Printf("error unmarshaling route from request body: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	if err := route.Validate(); err != nil {
		log.Printf("error validating dto: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	s.routes.Remove(route.From)

	s.workers.Add(1)

	go func() {
		defer s.workers.Done()

		if err := s.db.Where("`from` = ?", route.From).Delete(&models.Route{}).Error; err != nil { //nolint:exhaustivestruct
			log.Printf("error deleting route from db: %v", err)

			return
		}

		log.Printf("route deleted from db")
	}()
}

func (s Server) showDashboard(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.show404(rw, r)

		return
	}

	hosts := s.autocomplete.Hosts()

	if err := s.indexTemplate.Execute(rw, struct {
		Routes map[string]routing.RouteInfo
		Hosts  []string
	}{
		s.routes.GetAll(),
		hosts,
	}); err != nil {
		log.Printf("error executing template: %v", err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}
}

func api404(rw http.ResponseWriter, r *http.Request) {
	log.Printf("not found: %s", r.URL.Path)
	http.NotFound(rw, r)
}

func (s Server) show404(rw http.ResponseWriter, r *http.Request) {
	log.Printf("not found: %s", r.URL.Path)

	if err := s.notFoundTemplate.Execute(rw, nil); err != nil {
		log.Printf("error executing template: %v", err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}
}
