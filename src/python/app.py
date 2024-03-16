import os
import json
import boto3 
import requests
from decimal import Decimal

if os.environ.get('LOCAL') == "true":
    database = boto3.resource('dynamodb', endpoint_url="http://dynamodb-local:8000")
else:
    database = boto3.resource('dynamodb')
    
fuel_prices = database.Table("current_fuel_prices")

URI = "https://fppdirectapi-prod.safuelpricinginformation.com.au"


def convert_to_decimal(data):
    for i, site in enumerate(data):
        data[i]["Price"] = Decimal(site["Price"])
    return data

""" 
GetSitesPrices

send a request to get all prices from OUT.

returns an object containing a status code and the prices as a list of objects.
"""
def send_prices_req():
    res = requests.get(
        URI + "/Price/GetSitesPrices?countryId=21&geoRegionLevel=3&geoRegionId=4",
        headers = {
            "Authorization": API_KEY
        }
    )

    if res.status_code != 200:
        return res.status_code, []
    
    return 200, json.loads(res.text)["SitePrices"]


""" 
post prices to database

send all prices to be stored in the database to take load off the source API.
"""
def post_prices(price_list):
    # convert the float prices to decimal types.
    print("converting prices to decimal.")
    decimal_prices = convert_to_decimal(price_list)

    # batch write the items to the database.
    print("writing to database.")
    for i, item in enumerate(decimal_prices):
        with fuel_prices.batch_writer() as batch:
            if i % 25 == 0:
                print("%d items processed of %d." % (i, len(decimal_prices)))
                break
            res = batch.put_item(Item=item)

    print("success!")
    return res


def lambda_updateDatabase(ev, ctx):
    # get the prices from the SAFPIS API.
    print("sending request.")
    status, price_data = send_prices_req()

    # catch failed requests.
    if status != 200:
        return {
        "statusCode": status,
        "body": json.dumps({
            "message": "error while fetching data.",
        }),
    }

    # store the prices in the database.
    print("sending to database.")
    status = post_prices(price_data)
    return {
        "statusCode": 200,
        "body": json.dumps(status),
    }


""" example hello world route """
def lambda_handler(event, context):
    """Sample pure Lambda function

    Parameters
    ----------
    event: dict, required
        API Gateway Lambda Proxy Input Format

        Event doc: https://docs.aws.amazon.com/apigateway/latest/developerguide/set-up-lambda-proxy-integrations.html#api-gateway-simple-proxy-for-lambda-input-format

    context: object, required
        Lambda Context runtime methods and attributes

        Context doc: https://docs.aws.amazon.com/lambda/latest/dg/python-context-object.html

    Returns
    ------
    API Gateway Lambda Proxy Output Format: dict

        Return doc: https://docs.aws.amazon.com/apigateway/latest/developerguide/set-up-lambda-proxy-integrations.html
    """

    return {
        "statusCode": 200,
        "body": json.dumps({
            "message": "hello world",
            # "location": ip.text.replace("\n", "")
        }),
    }