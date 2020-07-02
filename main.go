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
		"CREATE TABLE IF NOT EXISTS domains (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), domain STRING, servers_changed bool, previous_ssl_grade string, logo string, title string, is_down bool )"); err != nil {
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

			if len(idm) > 0 {
				cmd := "whois " + endpoint.IpAddress
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

				fmt.Printf("found idm: %v\n", idm)
				if _, err := db.Exec("UPDATE servers SET domain_id=$1, address=$2, ssl_grade=$3, country=$4, owner=$5 WHERE id=$6", idn, endpoint.IpAddress, endpoint.Grade, country, organization, idm); err != nil {
					log.Fatal(err)
				}
			} else {
				cmd := "whois " + endpoint.IpAddress
				err, out, errout := Shellout(cmd)
				if err != nil {
					log.Printf("error: %v\n", err)
				}
				if errout != "" {
					log.Printf("error: %v\n", errout)
				}
				i := strings.Index(out, "Country")

				country := ""
				if i > -1 {
					country = out[i+15 : i+18]
				} else {
					fmt.Println("Index not found")
				}

				i = strings.Index(out, "OrgName")

				organization := ""
				if i > -1 {
					organization = out[i+16 : i+22]
				} else {
					fmt.Println("Index not found")
				}

				if _, err := db.Exec("INSERT INTO servers (domain_id, address, ssl_grade, country, owner) VALUES ($1, $2, $3, $4, $5)", idn, endpoint.IpAddress, endpoint.Grade, country, organization); err != nil {
					log.Fatal(err)
				}
			}
		}
		// respond to request
		returnInfo(ctx, idn)
	} else {
		// Insert domain into the "domains" table.
		tblname := "domains"
		quoted := pq.QuoteIdentifier(tblname)
		fmt.Printf("quoted: %v\n", quoted)
		if _, err := db.Exec("INSERT INTO domains (domain, servers_changed, previous_ssl_grade, logo, title, is_down) VALUES ($1, $2, $3, $4, $5, $6)", domain, false, "Previous Grade", "myLogo", "myTitle", false); err != nil {
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
			cmd := "whois " + endpoint.IpAddress
			err, out, errout := Shellout(cmd)
			if err != nil {
				log.Printf("error: %v\n", err)
			}
			if errout != "" {
				log.Printf("error: %v\n", errout)
			}
			i := strings.Index(out, "Country")

			country := ""
			if i > -1 {
				country = out[i+15 : i+18]
			} else {
				fmt.Println("Index not found")
			}

			i = strings.Index(out, "OrgName")

			organization := ""
			if i > -1 {
				organization = out[i+16 : i+22]
			} else {
				fmt.Println("Index not found")
			}

			if _, err := db.Exec("INSERT INTO servers (domain_id, address, ssl_grade, country, owner) VALUES ($1, $2, $3, $4, $5)", idn, endpoint.IpAddress, endpoint.Grade, country, organization); err != nil {
				log.Fatal(err)
			}
		}
		// respond to request
		returnInfo(ctx, idn)
	}
}

func returnInfo(ctx *fasthttp.RequestCtx, idn string) {
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
	var previous_ssl_grade string
	var logo string
	var title string
	var is_down bool
	var serverInfo ServerInfo
	sel := "SELECT * FROM domains WHERE id= $1"
	err = db.QueryRow(sel, idn).Scan(&id, &domain, &servers_changed, &previous_ssl_grade, &logo, &title, &is_down)
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

	serverInfo = ServerInfo{domain, servers, servers_changed, ssl_grade_to_report, previous_ssl_grade, logo, title, is_down}
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

func main() {
	router := fasthttprouter.New()
	router.GET("/getDomains", GetDomains)
	router.GET("/search/:domain", Search)

	log.Println("Listening on localhost:8000")

	log.Fatal(fasthttp.ListenAndServe(":8000", router.Handler))
}
