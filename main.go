package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"strings"

	"github.com/buaazp/fasthttprouter"
	"github.com/gocolly/colly/v2"
	"github.com/lib/pq"
	"github.com/valyala/fasthttp"
)

type Domain struct {
	Name string
}

type Server struct {
	Address   string
	Ssl_grade string
	Country   string
	Owner     string
}

type ServerInfo struct {
	Name               string
	Servers            []Server
	Servers_changed    bool
	Ssl_grade          string
	Previous_ssl_grade string
	Logo               string
	Title              string
	Is_down            bool
}

type LabsResponse struct {
	Endpoints []Endpoint
	Status    string
}

type Endpoint struct {
	IpAddress string
	Grade     string
}

func GetDomains(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
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
	ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
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
	// fmt.Println("URL:>", url)

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
	// fmt.Println("response Body:", string(body))

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
			// Get id if server already exists
			sel := "SELECT id FROM servers WHERE address=$1"
			var idm string
			err = db.QueryRow(sel, endpoint.IpAddress).Scan(&idm)
			if err != nil && err != sql.ErrNoRows {
				log.Fatal(err)
			}

			country, organization := getCountryOwner(endpoint.IpAddress)

			if len(idm) > 0 {

				fmt.Printf("found idm: %v\n", idm)
				if _, err := db.Exec("UPDATE servers SET domain_id=$1, address=$2, ssl_grade=$3, country=$4, owner=$5 WHERE id=$6", idn, endpoint.IpAddress, endpoint.Grade, country, organization, idm); err != nil {
					log.Fatal(err)
				}
			} else {

				if _, err := db.Exec("INSERT INTO servers (domain_id, address, ssl_grade, country, owner) VALUES ($1, $2, $3, $4, $5)", idn, endpoint.IpAddress, endpoint.Grade, country, organization); err != nil {
					log.Fatal(err)
				}
			}
		}

		//get Title
		title, logo := getTitleLogo(domain)

		if _, err := db.Exec("UPDATE domains SET title=$1, logo=$2 WHERE id=$3", title, logo, idn); err != nil {
			log.Fatal(err)
		}

		// Get previousGrade
		var previousGrade string
		sel := "SELECT ssl_grade FROM domains WHERE id=$1"
		err = db.QueryRow(sel, idn).Scan(&previousGrade)
		if err != nil && err != sql.ErrNoRows {
			log.Fatal(err)
		}

		// respond to request
		returnInfo(ctx, idn, previousGrade)

		//domain doesn't exist
	} else {
		// Insert domain into the "domains" table.
		tblname := "domains"
		quoted := pq.QuoteIdentifier(tblname)
		fmt.Printf("quoted: %v\n", quoted)

		isDown := true
		if labsResponse.Status == "Ready" || labsResponse.Status == "READY" {
			isDown = false
		}

		title, logo := getTitleLogo(domain)

		// add domain
		if _, err := db.Exec("INSERT INTO domains (domain, servers_changed, ssl_grade,previous_ssl_grade, logo, title, is_down) VALUES ($1, $2, $3, $4, $5, $6, $7)", domain, false, "to update", "no info", logo, title, isDown); err != nil {
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

			country, organization := getCountryOwner(endpoint.IpAddress)

			if _, err := db.Exec("INSERT INTO servers (domain_id, address, ssl_grade, country, owner) VALUES ($1, $2, $3, $4, $5)", idn, endpoint.IpAddress, endpoint.Grade, country, organization); err != nil {
				log.Fatal(err)
			}
		}
		// respond to request
		returnInfo(ctx, idn, "no info")
	}
}

func returnInfo(ctx *fasthttp.RequestCtx, idn string, previousGrade string) {
	// Connect to the "api_info" database.
	db, err := sql.Open("postgres",
		"postgresql://maxroach@localhost:26257/api_info?ssl=true&sslmode=require&sslrootcert=certs/ca.crt&sslkey=certs/client.maxroach.key&sslcert=certs/client.maxroach.crt")
	if err != nil {
		log.Fatal("error connecting to the database: ", err)
	}
	defer db.Close()

	// Return the domain info.
	var id string
	var domain string
	var servers_changed bool
	var ssl_grade string
	var previous_ssl_grade string
	var logo string
	var title string
	var is_down bool
	var serverInfo ServerInfo
	sel := "SELECT * FROM domains WHERE id= $1"
	err = db.QueryRow(sel, idn).Scan(&id, &domain, &servers_changed, &ssl_grade, &previous_ssl_grade, &logo, &title, &is_down)
	if err != nil && err != sql.ErrNoRows {
		log.Fatal(err)
	}

	// Return the servers info.
	rows, err := db.Query("SELECT * FROM servers where domain_id=$1", idn)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	grades := [8]string{"F", "E", "D", "C", "B", "A-", "A", "A+"}
	ssl_grade_to_report := grades[7]

	var servers []Server
	for rows.Next() {
		var id string
		var domain_id string
		var address string
		var ssl_grade string
		var country string
		var owner string
		if err := rows.Scan(&id, &domain_id, &address, &ssl_grade, &country, &owner); err != nil {
			log.Fatal(err)
		}
		if indexOf(ssl_grade, grades) < indexOf(ssl_grade_to_report, grades) {
			ssl_grade_to_report = grades[indexOf(ssl_grade, grades)]
		}
		servers = append(servers, Server{address, ssl_grade, country, owner})
	}

	// if domain doesn't exist
	if previousGrade == "no info" {
		if _, err := db.Exec("UPDATE domains SET ssl_grade=$1 WHERE id=$2", ssl_grade_to_report, idn); err != nil {
			log.Fatal(err)
		}
		// domain exists
	} else {
		if _, err := db.Exec("UPDATE domains SET ssl_grade=$1, previous_ssl_grade=$2 WHERE id=$3", ssl_grade_to_report, previousGrade, idn); err != nil {
			log.Fatal(err)
		}
	}

	serverInfo = ServerInfo{domain, servers, servers_changed, ssl_grade_to_report, previousGrade, logo, title, is_down}
	jsonInfo, _ := json.Marshal(serverInfo)
	fmt.Fprintf(ctx, "%s\n", jsonInfo)
}

func indexOf(element string, data [8]string) int {
	for k, v := range data {
		if element == v {
			return k
		}
	}
	return -1 //not found.
}

const ShellToUse = "bash"

func Shellout(command string) (error, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.Command(ShellToUse, "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return err, stdout.String(), stderr.String()
}

func getCountryOwner(ipAdd string) (string, string) {
	cmd := "whois " + ipAdd
	err, out, errout := Shellout(cmd)
	if err != nil {
		log.Printf("error: %v\n", err)
	}
	if errout != "" {
		log.Printf("error: %v\n", errout)
	}

	i := strings.Index(out, "Country:")
	country := ""
	if i > -1 {
		country = out[i+16 : i+18]
	} else {
		fmt.Println("Index not found")
	}

	i = strings.Index(out, "OrgName:")
	organization := ""
	if i > -1 {
		organization = out[i+16 : i+22]
	} else {
		fmt.Println("Index not found")
	}

	return country, organization
}

func getTitleLogo(domain string) (string, string) {

	title, logo := "", ""

	// Instantiate default collector
	c := colly.NewCollector()

	// On every a element which has href attribute call callback
	c.OnHTML("head title", func(e *colly.HTMLElement) {
		title = e.Text
	})

	// On every a element which has href attribute call callback
	c.OnHTML("head meta[content]", func(e *colly.HTMLElement) {
		myElem := e.Attr("content")
		if strings.Contains(myElem, ".png") && logo == "" {
			logo = myElem
		}
	})

	// Before making a request print "Visiting ..."
	// c.OnRequest(func(r *colly.Request) {
	// 	fmt.Println("Visiting", r.URL.String())
	// })

	// Start scraping on https://hackerspaces.org
	c.Visit("https://" + domain + "/")

	return title, logo
}

func main() {
	router := fasthttprouter.New()
	router.GET("/getDomains", GetDomains)
	router.GET("/search/:domain", Search)

	log.Println("Listening on localhost:8000")

	log.Fatal(fasthttp.ListenAndServe(":8000", router.Handler))
}
