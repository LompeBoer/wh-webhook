package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"

	"github.com/LompeBoer/wh-webhook/internal/whdiscord"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

const (
	RemoteScheme = "https"
	RemoteHost   = "discord.com"
	LocalScheme  = "http"
	LocalHost    = "localhost"
)

// NewProxy takes target host and creates a reverse proxy
func NewProxy(targetHost string) (*httputil.ReverseProxy, error) {
	url, err := url.Parse(targetHost)
	if err != nil {
		return nil, err
	}

	parser := whdiscord.NewParser(viper.GetString("messageStyle"))

	proxy := httputil.NewSingleHostReverseProxy(url)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		modifyRequest(req, parser)
	}
	// proxy.ModifyResponse = modifyResponse()
	proxy.ErrorHandler = errorHandler()

	return proxy, nil
}

func modifyRequest(req *http.Request, parser *whdiscord.Parser) {
	req.Host = RemoteHost
	if req.Body != nil {
		fmt.Print(".")

		// Read incoming body.
		b, err := io.ReadAll(req.Body)
		if err != nil {
			log.Printf("Failed to read body: %s\n", err.Error())
			return
		}

		// Rewrite body.
		newb := parser.ParseMessage(b)
		req.Body.Close()

		// Update body in original request.
		req.Body = ioutil.NopCloser(bytes.NewBuffer(newb))
		req.Header.Set("Content-Length", strconv.Itoa(len(newb)))
		req.ContentLength = int64(len(newb))
	}
}

func errorHandler() func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, req *http.Request, err error) {
		fmt.Printf("Got error while modifying response: %v \n", err)
	}
}

// func modifyResponse() func(*http.Response) error {
// 	return func(resp *http.Response) error {
// 		return errors.New("response body is invalid")
// 	}
// }

// ProxyRequestHandler handles the http request using proxy
func ProxyRequestHandler(proxy *httputil.ReverseProxy) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	}
}

func loadConfig() {
	viper.SetConfigName("wh-webhook")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error if desired
			log.Println("Config file not found, using default settings.")
		} else {
			// Config file was found but another error was produced
			log.Fatal(err)
		}
	}

	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		fmt.Println("Config file changed:", e.Name)
	})

	viper.SetDefault("messageStyle", "simple")
	viper.SetDefault("port", 8082)
}

func main() {
	log.Printf("Starting wh-webhook (v%s)\n\n", "0.1-alpha")
	loadConfig()
	remote := RemoteScheme + "://" + RemoteHost
	proxy, err := NewProxy(remote)
	if err != nil {
		log.Fatal(err)
	}

	port := viper.GetString("port")
	fmt.Printf("Replace \"%s\" with \"%s://%s:%s\" in WH bot settings.\n\n", remote, LocalScheme, LocalHost, port)

	http.HandleFunc("/", ProxyRequestHandler(proxy))
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
