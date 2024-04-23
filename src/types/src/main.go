package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
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
	isLocal         bool   = os.Getenv("local") == "true"
	isUpdatingSites bool   = os.Getenv("update_sites") == "true"
	apikey          string = os.Getenv("api_key")
)

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
	// switch request.Path {
	// case "/prices":
	// 	return handlePrices(request)
	// case "/sites":
	// 	return getAllSites(request)
	// case "/update":
	// 	return handlePricesUpdate(request)
	// }

	return respondWithStdErr(nil, "")
}

func handlePost(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
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
