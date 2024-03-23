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
	writeBatchSize  int    = 25
	readBatchSize   int    = 100
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
	SiteID             int     `json:"SiteId"`
	FuelID             int     `json:"FuelId"`
	CollectionMethod   string  `json:"CollectionMethod"`
	TransactionDateUTC string  `json:"TransactionDateUTC"`
	Price              float64 `json:"Price"`
}

type PetrolStationSite struct {
	SiteId int     `json:"SiteId"`
	Name   string  `json:"Name"`
	Lat    float64 `json:"Lat"`
	Lng    float64 `json:"Lng"`
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
		return respondWithStdErr(nil, "table doesn't exist.")
	}

	// get all sites
	// - send req
	fmt.Println("Getting all sites.")
	allSitesRaw, err := client.Scan(&dynamodb.ScanInput{
		TableName: aws.String(sitesTableName),
	})
	if err != nil {
		return respondWithStdErr(err, "")
	}

	// - trim
	fmt.Printf("Trimming all sites. %d items\n", len(allSitesRaw.Items))
	allSites := []PetrolStationSite{}
	for _, rawsite := range allSitesRaw.Items {
		name := *rawsite["N"].S

		SiteId, err := strconv.Atoi(*rawsite["SiteId"].N)
		if err != nil {
			return respondWithStdErr(err, "error while converting siteid into int.")
		}

		lat, err := strconv.ParseFloat(*rawsite["Lt"].N, 64)
		if err != nil {
			return respondWithStdErr(err, "error while converting lat into float.")
		}

		lng, err := strconv.ParseFloat(*rawsite["Lg"].N, 64)
		if err != nil {
			return respondWithStdErr(err, "error while converting long into float.")
		}

		site := PetrolStationSite{
			SiteId: SiteId,
			Name:   name,
			Lat:    float64(lat),
			Lng:    float64(lng),
		}

		allSites = append(allSites, site)
	}

	// - marshall
	fmt.Println("Marshalling all sites.")
	bytes, err := json.Marshal(allSites)
	if err != nil {
		return respondWithStdErr(err, "")
	}

	fmt.Println("Done!")
	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       string(bytes),
	}, nil
}

func respondWithStdErr(err error, errstring string) (events.APIGatewayProxyResponse, error) {
	if err == nil {
		return events.APIGatewayProxyResponse{
			Body:       errstring,
			StatusCode: 500,
		}, err
	}

	return events.APIGatewayProxyResponse{
		Body:       fmt.Sprintf("%s: %s", errstring, err.Error()),
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
		}, nil

	case "/sites":
		return getAllSites()
	}

	return respondWithStdErr(nil, "")
}

func handlePost(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// check the path and route based on that.
	switch request.Path {
	case "/prices":
		// get params
		fmt.Println("getting fuel type from params.")
		fuelId := request.QueryStringParameters["fuelType"]

		fmt.Println("getting sites from body.")
		var fuelSites []int
		err := json.Unmarshal([]byte(request.Body), &fuelSites)
		if err != nil {
			return respondWithStdErr(err, "")
		}

		fmt.Printf("getting prices for fuel type %s\n", fuelId)

		// get prices from DB.
		dbclient := getClient()
		if !checkTableExists(dbclient, pricesTableName) {
			return respondWithStdErr(nil, "prices table doesn't exist.")
		}

		// update the database.
		var item map[string]*dynamodb.AttributeValue

		allPrices := map[int]float64{}
		fmt.Printf("fetching %d prices from database.\n", len(fuelSites))
		for n := 0; n < len(fuelSites); {
			attrs := []map[string]*dynamodb.AttributeValue{}

			end := min(n+readBatchSize, len(fuelSites))
			for _, siteId := range fuelSites[n:end] {
				// - marshall the struct
				item = map[string]*dynamodb.AttributeValue{
					"SiteId": {N: aws.String(fmt.Sprintf("%d", siteId))},
					"FuelId": {N: aws.String(fuelId)},
				}

				// - append the write req
				attrs = append(attrs, item)
			}

			// - send the batch
			batchReq := dynamodb.BatchGetItemInput{
				RequestItems: map[string]*dynamodb.KeysAndAttributes{
					pricesTableName: {
						Keys: attrs,
					},
				},
			}
			batchRes, err := dbclient.BatchGetItem(&batchReq)
			if err != nil {
				fmt.Println("Error while sending batch get item.")
				return respondWithStdErr(err, "")
			}

			for _, item := range batchRes.Responses[pricesTableName] {
				// id, err := strconv.Atoi(strings.Split(*item["SiteId"].S, ":")[0])
				id, err := strconv.Atoi(*item["SiteId"].N)
				if err != nil {
					return respondWithStdErr(err, "error while converting siteid to int.")
				}

				prices, err := strconv.ParseFloat(*item["P"].N, 64)
				if err != nil {
					return respondWithStdErr(err, "error while converting price to float.")
				}

				allPrices[id] = prices
			}

			n += readBatchSize
			fmt.Printf("found ~%d/%d records in database.\n", end, len(allPrices))
		}
		fmt.Printf("done!.\n")

		// marshall the prices.
		body, err := json.Marshal(allPrices)
		if err != nil {
			return respondWithStdErr(err, "error while marshalling prices.")
		}

		return events.APIGatewayProxyResponse{
			StatusCode: 200,
			Body:       string(body),
		}, nil

	}

	return respondWithStdErr(nil, "invalid path.")
}

func handler(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var res events.APIGatewayProxyResponse
	var err error

	switch request.HTTPMethod {
	default:
		return events.APIGatewayProxyResponse{
			StatusCode: 400,
		}, nil

	case http.MethodOptions:
		return handleCors(request)

	case http.MethodGet:
		res, err = handleGet(request)
	case http.MethodPost:
		res, err = handlePost(request)
	}

	res.Headers =
		map[string]string{
			"Access-Control-Allow-Headers": "*",
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Methods": "OPTIONS,GET,POST",
		}
	return res, err
}

func main() {
	lambda.Start(handler)
}

//{"CollectionMethod":{"S":"T"},"FuelId":{"N":"2"},"Price":{"N":"2799"},"SiteId":{"N":"61577372"},"TransactionDateUtc":{"S":"2023-10-27T05:11:11.663"}}
