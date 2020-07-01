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

type Domain struct {
	Name string
}

type Server struct {
	Address   string
	Ssl_grade string
}

type ServerInfo struct {
	Name               string
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

func GetDomains(ctx *fasthttp.RequestCtx) {
	// Connect to the "api_info" database.
	db, err := sql.Open("postgres",
		"postgresql://maxroach@localhost:26257/api_info?ssl=true&sslmode=require&sslrootcert=certs/ca.crt&sslkey=certs/client.maxroach.key&sslcert=certs/client.maxroach.crt")
	if err != nil {
		log.Fatal("error connecting to the database: ", err)
	}
	defer db.Close()

	// Return the domains.
	rows, err := db.Query("SELECT domain FROM domains")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var domains []Domain
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			log.Fatal(err)
		}
		domains = append(domains, Domain{domain})
	}
	jsonInfo, _ := json.Marshal(domains)
	fmt.Fprintf(ctx, "%s\n", jsonInfo)
}

func Search(ctx *fasthttp.RequestCtx) {
	// fmt.Fprintf(ctx, "hello, %s!\n", ctx.UserValue("domain"))
	domain := ctx.UserValue("domain").(string)

	// Connect to the "api_info" database.
	db, err := sql.Open("postgres",
		"postgresql://maxroach@localhost:26257/api_info?ssl=true&sslmode=require&sslrootcert=certs/ca.crt&sslkey=certs/client.maxroach.key&sslcert=certs/client.maxroach.crt")
	if err != nil {
		log.Fatal("error connecting to the database: ", err)
	}
	defer db.Close()

	// Create the "domains" table.
	if _, err := db.Exec(
		"CREATE TABLE IF NOT EXISTS domains (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), domain STRING, servers_changed bool, ssl_grade string, previous_ssl_grade string, logo string, title string, is_down bool )"); err != nil {
		log.Fatal(err)
	}

	// Create the "servers" table.
	if _, err := db.Exec(
		"CREATE TABLE IF NOT EXISTS servers (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), domain_id UUID REFERENCES domains(id) ON DELETE CASCADE, address string, ssl_grade string, country string, owner string)"); err != nil {
		log.Fatal(err)
	}

	// Get info from SSL Labs
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

	// var servers []Server
	// for _, endpoint := range labsResponse.Endpoints {
	// 	servers = append(servers, Server{endpoint.IpAddress, endpoint.Grade})
	// }

	// var serverInfo ServerInfo
	// serverInfo.Servers = servers
	// jsonInfo, _ := json.Marshal(serverInfo)
	// fmt.Fprintf(ctx, "%s\n", jsonInfo)

	// Get id if domain already exists
	sel := "SELECT id FROM domains WHERE domain= $1"
	var idn string
	err = db.QueryRow(sel, domain).Scan(&idn)
	if err != nil && err != sql.ErrNoRows {
		log.Fatal(err)
	}

	if len(idn) > 0 {
		fmt.Printf("found idn: %v\n", idn)
		// Insert servers into the "servers" table.
		for _, endpoint := range labsResponse.Endpoints {
			if _, err := db.Exec("INSERT INTO servers (domain_id, address, ssl_grade) VALUES ($1, $2, $3)", idn, endpoint.IpAddress, endpoint.Grade); err != nil {
				log.Fatal(err)
			}
		}
	} else {
		// Insert domain into the "domains" table.
		tblname := "domains"
		quoted := pq.QuoteIdentifier(tblname)
		fmt.Printf("quoted: %v\n", quoted)
		if _, err := db.Exec("INSERT INTO domains (domain) VALUES ($1)", domain); err != nil {
			log.Fatal(err)
		}

		// Get id if domain already exists
		sel := "SELECT id FROM domains WHERE domain= $1"
		err = db.QueryRow(sel, domain).Scan(&idn)
		if err != nil && err != sql.ErrNoRows {
			log.Fatal(err)
		}
		fmt.Printf("created idn: %v\n", idn)

		// Insert servers into the "servers" table.
		for _, endpoint := range labsResponse.Endpoints {
			if _, err := db.Exec("INSERT INTO servers (domain_id, address, ssl_grade) VALUES ($1, $2, $3)", idn, endpoint.IpAddress, endpoint.Grade); err != nil {
				log.Fatal(err)
			}
		}
	}
}

func main() {
	router := fasthttprouter.New()
	router.GET("/getDomains", GetDomains)
	router.GET("/search/:domain", Search)

	log.Println("Listening on localhost:8000")

	log.Fatal(fasthttp.ListenAndServe(":8000", router.Handler))
}
