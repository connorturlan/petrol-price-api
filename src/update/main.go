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
	sitesTableName  string = "current_fuel_prices"
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

func createTable(client *dynamodb.DynamoDB) error {
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

func checkTableExists(client *dynamodb.DynamoDB) bool {
	awsTables, err := client.ListTables(&dynamodb.ListTablesInput{})
	if err != nil {
		return false
	}

	tables := []string{}
	for _, table := range awsTables.TableNames {
		tables = append(tables, *table)
	}

	return slices.Contains(tables, pricesTableName)
}

func respondWithStdErr(err error) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse{
		Body:       err.Error(),
		StatusCode: http.StatusInternalServerError,
	}, err
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

func handleGet(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// create the dynamo dbClient.
	dbClient := getClient()

	// validate the table exists.
	fmt.Println("checking table exists.")
	if !checkTableExists(dbClient) {
		createTable(dbClient)
		respondWithStdErr(nil)
	}

	// get the fuel prices.
	// - create the request.
	httpClient := &http.Client{}
	pricesEndpoint := fuelURL + "/Price/GetSitesPrices?countryId=21&geoRegionLevel=3&geoRegionId=4"
	req, err := http.NewRequest(http.MethodGet, pricesEndpoint, nil)
	if err != nil {
		fmt.Println("Error while creating http client.")
		return respondWithStdErr(err)
	}
	req.Header.Set("Authorization", apikey)

	// - read the body
	fmt.Printf("Sending request, apikey: %s\n", apikey)
	res, err := httpClient.Do(req)
	if err != nil {
		fmt.Println("Error while sending http request.")
		return respondWithStdErr(err)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatalln(err)
		return respondWithStdErr(err)
	}

	// - unmarshall the json
	var prices FuelPrices
	err = json.Unmarshal(body, &prices)
	if err != nil {
		fmt.Println("Error while unmarshalling fuel prices.")
		fmt.Println(body)
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
		fmt.Printf("updated %d/%d records in database.\n", n, len(allPrices))
	}
	fmt.Printf("done!.\n")

	// return.
	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusAccepted,
		Body:       fmt.Sprintf("%d records updated.\n", len(allPrices)),
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
