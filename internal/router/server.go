package router

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/iskorotkov/router/internal/models"
	"github.com/iskorotkov/router/internal/routing"
)

type Server struct {
	routes *routing.Cache
}

func NewServer(routes *routing.Cache) Server {
	return Server{routes: routes}
}

func (s Server) ListenAndServe(port int) {
	server := http.NewServeMux()
	server.HandleFunc("/", s.applyRoute)

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), server); err != nil {
		log.Printf("error in server: %v", err)

		return
	}
}

func (s Server) applyRoute(rw http.ResponseWriter, r *http.Request) {
	schema := "http"
	if r.TLS != nil {
		schema = "https"
	}

	origins, err := getAddressAliases(fmt.Sprintf("%s://%s", schema, r.RemoteAddr))
	if err != nil {
		log.Printf("error parsing remote address: %v", err)
		http.Error(rw, "", http.StatusInternalServerError)

		return
	}

	for _, origin := range origins {
		info, ok := s.routes.Get(origin)
		if !ok {
			continue
		}

		otherURL := fmt.Sprintf("%s://%s%s", schema, info.To, r.URL.Path)

		switch info.Type {
		case models.RouteTypeRedirect:
			http.Redirect(rw, r, otherURL, http.StatusTemporaryRedirect)
		case models.RouteTypeProxy:
			proxyRequest(rw, r, otherURL)
		default:
			log.Printf("unknown route type %q", info.Type)
			http.Error(rw, "", http.StatusInternalServerError)

			return
		}

		return
	}

	log.Printf("no route configured for host %q", r.RemoteAddr)
	rw.WriteHeader(http.StatusBadGateway)
}

func getAddressAliases(address string) ([]string, error) {
	parsed, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("error normalizing address %q: %w", address, err)
	}

	var results []string

	if parsed.Port() != "" {
		switch parsed.Hostname() {
		case "::1", "127.0.0.1", "localhost":
			results = append(results,
				fmt.Sprintf("[::1]:%s", parsed.Port()),
				fmt.Sprintf("127.0.0.1:%s", parsed.Port()),
				fmt.Sprintf("localhost:%s", parsed.Port()))
		default:
			results = append(results, parsed.Host)
		}
	}

	switch parsed.Hostname() {
	case "::1", "127.0.0.1", "localhost":
		results = append(results, "[::1]", "127.0.0.1", "localhost")
	default:
		results = append(results, parsed.Hostname())
	}

	return results, nil
}

func proxyRequest(rw http.ResponseWriter, r *http.Request, otherURL string) {
	req, err := http.NewRequestWithContext(context.Background(), r.Method, otherURL, r.Body)
	if err != nil {
		log.Printf("error creating request: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	req.Header = r.Header

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("error getting content at %q: %v", otherURL, err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	defer resp.Body.Close()

	for name, values := range resp.Header {
		for _, v := range values {
			rw.Header().Add(name, v)
		}
	}

	rw.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(rw, resp.Body); err != nil {
		log.Printf("error copying request body: %v", err)
		http.Error(rw, "", http.StatusInternalServerError)

		return
	}
}
