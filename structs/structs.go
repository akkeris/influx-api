package structs

type Provisionspec struct {
	Plan        string `json:"plan"`
	Billingcode string `json:"billingcode"`
}


type Influxdbspec struct {
	Name  string `json:"INFLUX_DB"`
        Url   string `json:"INFLUX_URL"`
        Username    string `json:"INFLUX_USERNAME"`
	Password string `json:"INFLUX_PASSWORD"`
}

