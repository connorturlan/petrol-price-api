package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

const (
	region          string = "ap-southeast-2"
	pricesTableName string = "current_fuel_prices"
	sitesTableName  string = "safpis_fuel_sites"
	batchSize       int    = 25
	fuelURL         string = "https://fppdirectapi-prod.safuelpricinginformation.com.au"
)

var (
	isLocal bool   = os.Getenv("local") == "true"
	apikey  string = os.Getenv("api_key")
)

type FuelPrices struct {
	Prices []FuelPrice `json:"SitePrices"`
}

type FuelPrice struct {
	SiteID             int     `json:"SiteID"`
	FuelID             int     `json:"FuelID"`
	CollectionMethod   string  `json:"CollectionMethod"`
	TransactionDateUTC string  `json:"TransactionDateUTC"`
	Price              float64 `json:"Price"`
}

type PetrolStationSite struct {
	Name string  `json:"Name"`
	Lat  float64 `json:"Lat"`
	Lng  float64 `json:"Lng"`
}

func getClient() *dynamodb.DynamoDB {
	config := aws.NewConfig().WithRegion(region)
	if isLocal {
		fmt.Println("Using local endpoint.")
		config = config.WithEndpoint("http://dynamodb-local:8000")
	}

	session, err := session.NewSession()
	if err != nil {
		return nil
	}

	return dynamodb.New(session, config)
}

func checkTableExists(client *dynamodb.DynamoDB, tableName string) bool {
	awsTables, err := client.ListTables(&dynamodb.ListTablesInput{})
	if err != nil {
		return false
	}

	tables := []string{}
	for _, table := range awsTables.TableNames {
		tables = append(tables, *table)
	}

	return slices.Contains(tables, tableName)
}

func getAllSites() (events.APIGatewayProxyResponse, error) {
	// get dbclient
	client := getClient()

	if !checkTableExists(client, sitesTableName) {
		return respondWithStdErr(nil)
	}

	// get all sites
	// - send req
	fmt.Println("Getting all sites.")
	allSitesRaw, err := client.Scan(&dynamodb.ScanInput{
		TableName: aws.String(sitesTableName),
	})
	if err != nil {
		return respondWithStdErr(err)
	}

	// - trim
	fmt.Printf("Trimming all sites. %d items\n", len(allSitesRaw.Items))
	allSites := []PetrolStationSite{}
	for _, rawsite := range allSitesRaw.Items {
		name := *rawsite["N"].S

		lat, err := strconv.ParseFloat(*rawsite["Lt"].N, 64)
		if err != nil {
			return respondWithStdErr(err)
		}

		lng, err := strconv.ParseFloat(*rawsite["Lg"].N, 64)
		if err != nil {
			return respondWithStdErr(err)
		}

		site := PetrolStationSite{
			Name: name,
			Lat:  float64(lat),
			Lng:  float64(lng),
		}

		allSites = append(allSites, site)
	}

	// - marshall
	fmt.Println("Marshalling all sites.")
	bytes, err := json.Marshal(allSites)
	if err != nil {
		return respondWithStdErr(err)
	}

	fmt.Println("Done!")
	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       string(bytes),
	}, nil
}

func respondWithStdErr(err error) (events.APIGatewayProxyResponse, error) {
	if err == nil {
		return events.APIGatewayProxyResponse{
			Body:       "an error occured.",
			StatusCode: 500,
		}, err
	}

	return events.APIGatewayProxyResponse{
		Body:       err.Error(),
		StatusCode: 500,
	}, err
}

func handleCors(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Access-Control-Allow-Headers": "*",
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Methods": "OPTIONS,GET,POST",
		},
	}, nil
}

func handleGet(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// check the path and route based on that.
	switch request.Path {
	case "/prices":
		latitude := request.QueryStringParameters["lat"]
		longitude := request.QueryStringParameters["long"]
		fuelType := request.QueryStringParameters["fuelType"]

		return events.APIGatewayProxyResponse{
			StatusCode: 200,
			Body:       fmt.Sprintf("Prices Look Good! pos@%s.%s for %s\n", latitude, longitude, fuelType),
			Headers: map[string]string{
				"Access-Control-Allow-Headers": "*",
				"Access-Control-Allow-Origin":  "*",
				"Access-Control-Allow-Methods": "OPTIONS,GET,POST",
			},
		}, nil

	case "/sites":
		return getAllSites()

		// latitude := request.QueryStringParameters["lat"]
		// longitude := request.QueryStringParameters["long"]

		// return events.APIGatewayProxyResponse{
		// 	StatusCode: 200,
		// 	Body:       fmt.Sprintf("Sites Look Good! pos@%s.%s\n", latitude, longitude),
		// 	Headers: map[string]string{
		// 		"Access-Control-Allow-Headers": "*",
		// 		"Access-Control-Allow-Origin":  "*",
		// 		"Access-Control-Allow-Methods": "OPTIONS,GET,POST",
		// 	},
		// }, nil

	default:
		return respondWithStdErr(nil)
	}

	// // create the dynamo dbClient.
	// dbClient := getClient()

	// // validate the table exists.
	// fmt.Println("checking table exists.")
	// if !checkTableExists(dbClient) {
	// 	createTable(dbClient)
	// 	respondWithStdErr(nil)
	// }

	// // return.
	// return events.APIGatewayProxyResponse{
	// 	StatusCode: 200,
	// 	Body:       fmt.Sprintf("Looks Good! Apikey: %s\n", apikey),
	// 	Headers: map[string]string{
	// 		"Access-Control-Allow-Headers": "*",
	// 		"Access-Control-Allow-Origin":  "*",
	// 		"Access-Control-Allow-Methods": "OPTIONS,GET,POST",
	// 	},
	// }, nil
}

func handler(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	switch request.HTTPMethod {
	case http.MethodOptions:
		return handleCors(request)
	case http.MethodGet:
		res, err := handleGet(request)
		res.Headers =
			map[string]string{
				"Access-Control-Allow-Headers": "*",
				"Access-Control-Allow-Origin":  "*",
				"Access-Control-Allow-Methods": "OPTIONS,GET,POST",
			}

		return res, err
	default:
		return events.APIGatewayProxyResponse{
			StatusCode: 400,
		}, nil
	}
}

func main() {
	lambda.Start(handler)
}

//{"CollectionMethod":{"S":"T"},"FuelId":{"N":"2"},"Price":{"N":"2799"},"SiteId":{"N":"61577372"},"TransactionDateUtc":{"S":"2023-10-27T05:11:11.663"}}
