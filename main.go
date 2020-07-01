package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/buaazp/fasthttprouter"
	"github.com/lib/pq"
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

	// Connect to the "api_info" database.
	db, err := sql.Open("postgres",
		"postgresql://maxroach@localhost:26257/api_info?ssl=true&sslmode=require&sslrootcert=certs/ca.crt&sslkey=certs/client.maxroach.key&sslcert=certs/client.maxroach.crt")
	if err != nil {
		log.Fatal("error connecting to the database: ", err)
	}
	defer db.Close()

	// Create the "servers" table.
	if _, err := db.Exec(
		"CREATE TABLE IF NOT EXISTS servers (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), domain STRING )"); err != nil {
		log.Fatal(err)
	}

	// Insert domain into the "servers" table.
	tblname := "servers"
	quoted := pq.QuoteIdentifier(tblname)
	if _, err := db.Exec(
		fmt.Sprintf("INSERT INTO %s (domain) VALUES ($1)", quoted), domain); err != nil {
		log.Fatal(err)
	}

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

	log.Println("Listening on localhost:8000")

	log.Fatal(fasthttp.ListenAndServe(":8000", router.Handler))
}
