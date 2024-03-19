package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"slices"

	"github.com/shopspring/decimal"

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
	isLocal         bool   = os.Getenv("local") == "true"
	isUpdatingSites bool   = os.Getenv("update_sites") == "true"
	apikey          string = os.Getenv("api_key")
)

type FuelPriceList struct {
	Prices []FuelPrice `json:"SitePrices"`
}

type FuelPrice struct {
	SiteID             int     `json:"SiteID"`
	FuelID             int     `json:"FuelID"`
	CollectionMethod   string  `json:"CollectionMethod"`
	TransactionDateUTC string  `json:"TransactionDateUTC"`
	Price              float64 `json:"Price"`
}

type PetrolStationList struct {
	Sites []PetrolStationSite `json:"S"`
}

type PetrolStationSite struct {
	SiteID    int     `json:"S"`
	Address   string  `json:"A"`
	Name      string  `json:"N"`
	BrandID   int     `json:"B"`
	Postcode  string  `json:"P"`
	Latitude  float64 `json:"Lat"`
	Longitude float64 `json:"Lng"`
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

func createPriceTable(client *dynamodb.DynamoDB) error {
	fmt.Println("Creating new table!")

	_, err := client.CreateTable(&dynamodb.CreateTableInput{
		TableName: aws.String(pricesTableName),
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: aws.String("SiteId"),
				AttributeType: aws.String("N"),
			},
			{
				AttributeName: aws.String("FuelId"),
				AttributeType: aws.String("N"),
			},
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String("SiteId"),
				KeyType:       aws.String("HASH"),
			},
			{
				AttributeName: aws.String("FuelId"),
				KeyType:       aws.String("RANGE"),
			},
		},
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(1),
			WriteCapacityUnits: aws.Int64(1),
		},
	})

	return err
}

func createSiteTable(client *dynamodb.DynamoDB) error {
	fmt.Println("Creating new table!")

	_, err := client.CreateTable(&dynamodb.CreateTableInput{
		TableName: aws.String(sitesTableName),
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: aws.String("SiteId"),
				AttributeType: aws.String("N"),
			},
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String("SiteId"),
				KeyType:       aws.String("HASH"),
			},
		},
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(1),
			WriteCapacityUnits: aws.Int64(1),
		},
	})

	return err
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

func respondWithStdErr(err error) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse{
		Body:       err.Error(),
		StatusCode: http.StatusInternalServerError,
	}, err
}

func sendJsonRequest[T interface{}](url string, obj *T) error {
	httpClient := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		fmt.Println("Error while creating http client.")
		return err
	}
	req.Header.Set("Authorization", apikey)

	// - read the body
	fmt.Printf("Sending request, apikey: %s\n", apikey)
	res, err := httpClient.Do(req)
	if err != nil {
		fmt.Println("Error while sending http request.")
		return err
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatalln(err)
		return err
	}

	// - unmarshall the json
	err = json.Unmarshal(body, &obj)
	if err != nil {
		fmt.Println("Error while unmarshalling json body.")
		fmt.Println(string(body))
		return err
	}
	return nil
}

func handleCors(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Access-Control-Allow-Headers": "*",
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Methods": "OPTIONS,GET,POST",
		},
	}, nil
}

func getAllPrices(dbClient *dynamodb.DynamoDB) (events.APIGatewayProxyResponse, error) {
	// validate the table exists.
	fmt.Println("checking prices table exists.")
	if !checkTableExists(dbClient, pricesTableName) {
		createPriceTable(dbClient)
	}

	// get the fuel prices.
	// - create the request.
	var prices FuelPriceList
	pricesEndpoint := fuelURL + "/Price/GetSitesPrices?countryId=21&geoRegionLevel=3&geoRegionId=4"
	err := sendJsonRequest(pricesEndpoint, &prices)
	if err != nil {
		return respondWithStdErr(err)
	}

	// update the database.
	var item map[string]*dynamodb.AttributeValue

	allPrices := prices.Prices
	fmt.Printf("updating %d records in database.\n", len(allPrices))
	for n := 0; n < len(allPrices); {
		var writeReqs []*dynamodb.WriteRequest

		end := min(n+batchSize, len(allPrices))
		for _, price := range allPrices[n:end] {
			// - marshall the struct
			item = map[string]*dynamodb.AttributeValue{
				"SiteId": {N: aws.String(fmt.Sprintf("%d", price.SiteID))},
				"FuelId": {N: aws.String(fmt.Sprintf("%d", price.FuelID))},
				"M":      {S: aws.String(price.CollectionMethod)},
				"D":      {S: aws.String(price.TransactionDateUTC)},
				"P":      {N: aws.String(decimal.NewFromFloat(price.Price).String())},
			}

			// - append the write req
			writeReqs = append(writeReqs, &dynamodb.WriteRequest{PutRequest: &dynamodb.PutRequest{Item: item}})
		}

		// - send the batch
		batchReq := dynamodb.BatchWriteItemInput{RequestItems: map[string][]*dynamodb.WriteRequest{pricesTableName: writeReqs}}
		if _, err = dbClient.BatchWriteItem(&batchReq); err != nil {
			fmt.Println("Error while sending batch write item.")
			return respondWithStdErr(err)
		}

		n += batchSize
		fmt.Printf("updated ~%d/%d records in database.\n", end, len(allPrices))
	}
	fmt.Printf("done!.\n")

	return events.APIGatewayProxyResponse{}, nil
}

func getAllSites(dbClient *dynamodb.DynamoDB) (events.APIGatewayProxyResponse, error) {
	// validate the table exists.
	fmt.Println("checking sites table exists.")
	if !checkTableExists(dbClient, sitesTableName) {
		createSiteTable(dbClient)
	}

	// get the sites date.
	// - create the request.
	var sites PetrolStationList
	sitesEndpoint := fuelURL + "/Subscriber/GetFullSiteDetails?countryId=21&geoRegionLevel=3&geoRegionId=4"
	err := sendJsonRequest(sitesEndpoint, &sites)
	if err != nil {
		return respondWithStdErr(err)
	}

	// update the database.
	var item map[string]*dynamodb.AttributeValue

	allSites := sites.Sites
	fmt.Printf("updating %d records in database.\n", len(allSites))
	for n := 0; n < len(allSites); {
		var writeReqs []*dynamodb.WriteRequest

		end := min(n+batchSize, len(allSites))
		for _, petrolStation := range allSites[n:end] {
			// - marshall the struct
			item = map[string]*dynamodb.AttributeValue{
				"SiteId": {N: aws.String(fmt.Sprintf("%d", petrolStation.SiteID))},
				"A":      {S: aws.String(petrolStation.Address)},
				"N":      {S: aws.String(petrolStation.Name)},
				"B":      {N: aws.String(fmt.Sprintf("%d", petrolStation.BrandID))},
				"P":      {S: aws.String(petrolStation.Postcode)},
				"Lt":     {N: aws.String(decimal.NewFromFloat(petrolStation.Latitude).String())},
				"Lg":     {N: aws.String(decimal.NewFromFloat(petrolStation.Longitude).String())},
			}

			// - append the write req
			writeReqs = append(writeReqs, &dynamodb.WriteRequest{PutRequest: &dynamodb.PutRequest{Item: item}})
		}

		// - send the batch
		batchReq := dynamodb.BatchWriteItemInput{RequestItems: map[string][]*dynamodb.WriteRequest{sitesTableName: writeReqs}}
		if _, err = dbClient.BatchWriteItem(&batchReq); err != nil {
			fmt.Println("Error while sending batch write item.")
			return respondWithStdErr(err)
		}

		n += batchSize
		fmt.Printf("updated ~%d/%d records in database.\n", end, len(allSites))
	}
	fmt.Printf("done!.\n")

	return events.APIGatewayProxyResponse{}, nil
}

func handleGet(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var err error

	// create the dynamo dbClient.
	dbClient := getClient()

	_, err = getAllPrices(dbClient)
	if err != nil {
		return respondWithStdErr(err)
	}

	if isUpdatingSites {
		_, err = getAllSites(dbClient)
		if err != nil {
			return respondWithStdErr(err)
		}
	}

	// return.
	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusAccepted,
		Headers: map[string]string{
			"Access-Control-Allow-Headers": "*",
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Methods": "OPTIONS,GET,POST",
		},
	}, nil
}

func handler(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	switch request.HTTPMethod {
	case http.MethodOptions:
		return handleCors(request)
	case http.MethodGet:
		return handleGet(request)
	default:
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
		}, nil
	}
}

func main() {
	lambda.Start(handler)
}

//{"CollectionMethod":{"S":"T"},"FuelId":{"N":"2"},"Price":{"N":"2799"},"SiteId":{"N":"61577372"},"TransactionDateUtc":{"S":"2023-10-27T05:11:11.663"}}
