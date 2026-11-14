// API gateway: the single entry point (http://localhost:8080/api/v1) from openapi.yaml.
//
//	/api/v1/transactions/pay[...]  -> payment-service
//	everything else under /api/v1  -> core-service
//
// /internal/* on core-service is never routed from here.
package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"
)

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func proxy(raw string) http.Handler {
	u, err := url.Parse(raw)
	if err != nil {
		log.Fatalf("bad upstream %q: %v", raw, err)
	}
	p := httputil.NewSingleHostReverseProxy(u)
	p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("upstream error %s: %v", r.URL.Path, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream service unavailable"}`))
	}
	return p
}

func main() {
	core := proxy(getenv("CORE_SERVICE_URL", "http://localhost:8081"))
	pay := proxy(getenv("PAYMENT_SERVICE_URL", "http://localhost:8082"))

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/api/v1/transactions/pay", pay)
	mux.Handle("/api/v1/transactions/pay/", pay)
	mux.Handle("/api/v1/", core)

	srv := &http.Server{
		Addr:              ":" + getenv("PORT", "8080"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("gateway listening on %s", srv.Addr)
	log.Fatal(srv.ListenAndServe())
}
