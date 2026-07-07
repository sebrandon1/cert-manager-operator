package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	utils "github.com/openshift/cert-manager-operator/cmd/http01-proxy/pkg"
)

const defaultPort = "8888"

func main() {
	ctx := context.Background()

	env, err := utils.GetOCPEnvDetails(ctx)
	if err != nil {
		log.Fatalf("Error getting OCP environment details: %v", err)
	}
	log.Printf("OCP API: %s, API VIP: %s, APPS VIP: %s, Platform: %s, Version: %s",
		env.APIHostname, env.APIVIP, env.AppsVIP, env.PlatformType, env.ClusterVersion)

	if err := utils.SupportedOCPVersion(env.ClusterVersion); err != nil {
		log.Fatalf("Detected non-supported version: %v", err)
	}

	if env.AppsVIP == env.APIVIP {
		log.Printf("API VIP and APPS VIP are equal, no proxy needed")
		os.Exit(0)
	}

	port := os.Getenv("PROXY_PORT")
	if port == "" {
		port = defaultPort
	}
	if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
		log.Fatalf("Invalid PROXY_PORT %q: must be a number between 1 and 65535", port)
	}

	if err := utils.CreateNFTablesRuleMachineConfig(ctx, env.Client, env.APIVIP, port); err != nil {
		log.Fatalf("Error creating nft rules machineconfig: %v", err)
	}
	log.Println("NFTables Rules MachineConfig created/updated")

	backendServer := "http://" + env.AppsVIP + ":80"
	proxy, err := utils.NewReverseProxy(backendServer)
	if err != nil {
		log.Fatalf("Error creating reverse proxy: %v", err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
			log.Printf("Forwarding request to APPS VIP: %s", r.URL.Path)
			proxy.ServeHTTP(w, r)
		} else {
			http.Error(w, "Forbidden: Only /.well-known/acme-challenge/* is allowed", http.StatusForbidden)
		}
	})

	server := &http.Server{
		Addr:              ":" + port,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("Reverse proxy listening on :%s, forwarding http01 challenges for %s to %s", port, env.APIHostname, backendServer)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Error starting proxy: %v", err)
	}
}
