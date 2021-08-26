package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/cbuto/contrast-agent-injector/pkg/webhooks"
	log "github.com/sirupsen/logrus"
)

// WebhookServerParams is a struct containing the configuration for the webhook HTTP server
type WebhookServerParams struct {
	Port       int
	CertFile   string
	KeyFile    string
	SecretName string
}

func livenessHandler(response http.ResponseWriter, request *http.Request) {
	data := []byte("alive")
	if _, err := response.Write(data); err != nil {
		log.Error("Could not write response: ", err)
	}
}

func main() {
	var params WebhookServerParams
	flag.IntVar(&params.Port, "port", 8443, "Webhook server port.")
	flag.StringVar(&params.CertFile, "tlsCertFile", "/etc/webhook/certs/cert.pem", "File containing the x509 Certificate")
	flag.StringVar(&params.KeyFile, "tlsKeyFile", "/etc/webhook/certs/key.pem", "File containing the x509 private key for the certificate")
	flag.StringVar(&params.SecretName, "secretName", "", "Kubernetes secret containing the contrast_security.yaml file")
	flag.Parse()

	if len(params.SecretName) == 0 {
		log.Fatal("--secretName required")
	}
	log.SetFormatter(&log.JSONFormatter{})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)

	pair, err := tls.LoadX509KeyPair(params.CertFile, params.KeyFile)
	if err != nil {
		log.Fatal("Failed to load key pair: ", err)
	}

	mutateConfig := &webhooks.MutateConfig{
		SecretName: params.SecretName,
	}

	server := &http.Server{
		Addr:      fmt.Sprintf(":%v", params.Port),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/live", livenessHandler)
	mux.HandleFunc("/mutate", mutateConfig.MutateHandler)
	server.Handler = mux
	log.Info("Starting webhook server on port: ", params.Port)

	if err := server.ListenAndServeTLS("", ""); err != nil {
		log.Fatal("Failed to start webhook server: ", err)
	}
}
