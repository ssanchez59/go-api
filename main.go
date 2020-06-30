package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/buaazp/fasthttprouter"
	"github.com/valyala/fasthttp"
)

type Server struct {
	Address   string
	Ssl_grade string
}

type ServerInfo struct {
	Servers            []Server
	Servers_changed    bool
	Ssl_grade          string
	Previous_ssl_grade string
	Logo               string
	title              string
	Is_down            bool
}

type LabsResponse struct {
	Endpoints []Endpoint
}

type Endpoint struct {
	IpAddress string
	Grade     string
}

func Index(ctx *fasthttp.RequestCtx) {
	fmt.Fprint(ctx, "Welcome!\n")
}

func Hello(ctx *fasthttp.RequestCtx) {
	// fmt.Fprintf(ctx, "hello, %s!\n", ctx.UserValue("domain"))
	domain := ctx.UserValue("domain").(string)

	url := "https://api.ssllabs.com/api/v3/analyze?host=" + domain
	fmt.Println("URL:>", url)

	var jsonStr = []byte(`{"title":"Buy cheese and bread for breakfast."}`)
	req, err := http.NewRequest("GET", url, bytes.NewBuffer(jsonStr))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	// fmt.Println("response Status:", resp.Status)
	// fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))

	var labsResponse LabsResponse
	json.Unmarshal([]byte(body), &labsResponse)
	// fmt.Printf("Enpoints: %s", labsResponse.Endpoints[1].IpAddress)

	var servers []Server
	for _, endpoint := range labsResponse.Endpoints {
		servers = append(servers, Server{endpoint.IpAddress, endpoint.Grade})
	}

	var serverInfo ServerInfo
	serverInfo.Servers = servers
	jsonInfo, _ := json.Marshal(serverInfo)
	fmt.Fprintf(ctx, "%s\n", jsonInfo)
}

func main() {
	router := fasthttprouter.New()
	router.GET("/", Index)
	router.GET("/hello/:domain", Hello)

	log.Println("Listening on localhost:8080")

	log.Fatal(fasthttp.ListenAndServe(":8080", router.Handler))
}
