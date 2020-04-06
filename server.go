package main

import (
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"github.com/go-martini/martini"
	_ "github.com/lib/pq"
	"github.com/martini-contrib/binding"
	"github.com/martini-contrib/render"
	"github.com/nu7hatch/gouuid"
	structs "influx-api/structs"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

var brokerdb *sql.DB
var key []byte
var brokerdburl string
var influx_url string
var influx_username string
var influx_password string

func main() {
	initSecrets()
	brokerdb = initdb(brokerdburl)
	m := martini.Classic()
	m.Use(render.Renderer())
	m.Get("/v1/influxdb/plans", plans)
	m.Post("/v1/influxdb/instance", binding.Json(structs.Provisionspec{}), provision)
	m.Get("/v1/influxdb/url/:name", url)
	m.Delete("/v1/influxdb/instance/:name", delete)
	m.Run()
}

func initSecrets() {
        brokerdburl = os.Getenv("DATABASE_URL")
	influx_url = os.Getenv("INFLUX_URL")
	influx_username = os.Getenv("DATABASE_USERNAME")
	influx_password = os.Getenv("DATABASE_PASSWORD")
	key = []byte(os.Getenv("DATABASE_KEY"))
}
func returnmessage(r render.Render, code int, message string) {
	r.JSON(code, map[string]string{"message": message})
	return
}

func url(params martini.Params, r render.Render) {
	name, username, password, err := retrieve(params["name"])
	if err != nil {
		fmt.Println(err)
		returnmessage(r, 500, err.Error())
		return
	}
	var influxdbspec structs.Influxdbspec
	influxdbspec.Name = name
	influxdbspec.Url = influx_url
	influxdbspec.Username = username
	influxdbspec.Password = password
	r.JSON(200, influxdbspec)
}

func provision(spec structs.Provisionspec, berr binding.Errors, r render.Render) {
	var influxdbspec structs.Influxdbspec
	name := "i" + getUUID()
	user := "u" + getUUID()
	password := "p" + getUUID()
	err := executeCmd("create database " + name)
	if err != nil {
		fmt.Println(err)
		returnmessage(r, 500, err.Error())
		return
	}
	err = executeCmd("CREATE USER " + user + " with password '" + password + "'")
	if err != nil {
		fmt.Println(err)
		returnmessage(r, 500, err.Error())
		return
	}
	err = executeCmd("grant ALL on " + name + " to " + user + "")
	if err != nil {
		fmt.Println(err)
		returnmessage(r, 500, err.Error())
		return
	}
	influxdbspec.Name = name
	influxdbspec.Url = influx_url
	influxdbspec.Username = user
	influxdbspec.Password = password
	store(name, user, password, spec.Billingcode)
	r.JSON(200, influxdbspec)
}

func executeCmd(cmd string) (e error) {
	client := http.Client{}
	req, err := http.NewRequest("POST", influx_url+"/query", nil)
	q := req.URL.Query()
	q.Add("q", cmd)
	req.URL.RawQuery = q.Encode()

	if err != nil {
		fmt.Println(err)
		return err
	}
	req.SetBasicAuth(influx_username, influx_password)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending request")
		fmt.Println(err)
		return err
	}
	defer resp.Body.Close()

	bodybytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error sending request")
		fmt.Println(err)
		return err
	}
	fmt.Println(string(bodybytes))
	return nil
}

func getUUID() string {

	u, _ := uuid.NewV4()
	new := strings.Split(u.String(), "-")[0]
	return new
}

func createDB(db *sql.DB) (e error) {
	buf, err := ioutil.ReadFile("./create.sql")
	if err != nil {
		buf, err = ioutil.ReadFile("../create.sql")
		if err != nil {
			fmt.Println("Error: Unable to run migration scripts, could not load create.sql.")
			os.Exit(1)
		}
	}
	_, err = db.Query(string(buf))
	if err != nil {
		fmt.Println("Error: Unable to run migration scripts, execution failed.")
		fmt.Println(err)
		os.Exit(1)
	}
	return nil
}

func initdb(brokerdburl string) (db *sql.DB) {
	db, dberr := sql.Open("postgres", brokerdburl+"?sslmode=disable")
	if dberr != nil {
		fmt.Println(dberr)
		os.Exit(1)
	}
	db.SetMaxIdleConns(4)
	db.SetMaxOpenConns(20)
	createDB(db)
	return db
}

func store(name string, username string, password string, billingcode string) (e error) {
	var newname string
	err := brokerdb.QueryRow("INSERT INTO provision(name, username, password, billingcode) VALUES($1,$2,$3,$4) returning name;", name, username, stringencrypt(password), billingcode).Scan(&newname)

	if err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

func stringencrypt(plaintext string) (t string) {

	text := []byte(plaintext)
	block, err := aes.NewCipher(key)
	if err != nil {
		fmt.Println(err)
	}
	ciphertext := make([]byte, aes.BlockSize+len(text))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(crand.Reader, iv); err != nil {
		fmt.Println(err)
	}
	cfb := cipher.NewCFBEncrypter(block, iv)
	cfb.XORKeyStream(ciphertext[aes.BlockSize:], text)
	return base64.StdEncoding.EncodeToString(ciphertext)
}

func retrieve(name string) (n string, u string, p string, e error) {
	stmt, err := brokerdb.Prepare("select username, password from provision where name = $1 ")
	if err != nil {
		fmt.Println(err)
		return "", "", "", err
	}
	defer stmt.Close()
	rows, err := stmt.Query(name)
	defer rows.Close()
	var username string
	var password string
	for rows.Next() {
		err := rows.Scan(&username, &password)
		if err != nil {
			fmt.Println(err)
			return "", "", "", err
		}
	}
	return name, username, stringdecrypt(password), nil
}

func stringdecrypt(b64 string) (t string) {
	text, _ := base64.StdEncoding.DecodeString(b64)
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	if len(text) < aes.BlockSize {
		panic("ciphertext too short")
	}
	iv := text[:aes.BlockSize]
	text = text[aes.BlockSize:]
	cfb := cipher.NewCFBDecrypter(block, iv)
	cfb.XORKeyStream(text, text)
	return string(text)
}

func plans(params martini.Params, r render.Render) {
	plans := make(map[string]interface{})
	plans["shared"] = "Shared Mult-tenant db with default settings"
	r.JSON(200, plans)
}

func delete(params martini.Params, r render.Render) {
	name := params["name"]
	_, username, _, err := retrieve(name)
	if err != nil {
	}
	err = executeCmd("drop user " + username)
	if err != nil {
		fmt.Println(err)
		returnmessage(r, 500, err.Error())
		return
	}
	err = executeCmd("drop database " + name)
	if err != nil {
		fmt.Println(err)
		returnmessage(r, 500, err.Error())
		return
	}
	remove(name)
	returnmessage(r, 200, "deleted")
	return
}

func remove(name string) (e error) {
	stmt, err := brokerdb.Prepare("delete from provision where name=$1")
	if err != nil {
		fmt.Println(err)
		return err
	}
	res, err := stmt.Exec(name)
	if err != nil {
		fmt.Println(err)
		return err
	}
	affect, err := res.RowsAffected()
	if err != nil {
		fmt.Println(err)
		return err
	}
	fmt.Println(affect, "rows changed")
	return nil
}
