package main

import (
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

func (pricesList SA_FuelPriceList) ToPriceList() (FuelPriceList, error) {
	prices := FuelPriceList{
		Sites: map[int]FuelStation{},
	}

	for _, price := range pricesList.Prices {
		// check the petrol station exists.
		siteId := price.SiteId
		site, ok := prices.Sites[siteId]
		if !ok {
			site = FuelStation{
				SiteID:    siteId,
				FuelTypes: map[int]FuelPrice{},
			}
			prices.Sites[siteId] = site
		}

		// check the fuel record exists.
		fuelId := price.FuelId
		price := FuelPrice{
			FuelID:             fuelId,
			CollectionMethod:   price.CollectionMethod,
			TransactionDateUTC: price.TransactionDateUTC,
			Price:              int(price.Price),
		}
		site.FuelTypes[fuelId] = price
	}

	return prices, nil
}

// FuelPrice.Marshal returns a dynamodb representation of the FuelPrice struct.
func (price FuelPrice) Marshal() (map[string]*dynamodb.AttributeValue, error) {
	return map[string]*dynamodb.AttributeValue{
		"FuelId": {
			N: aws.String(fmt.Sprintf("%d", price.FuelID)),
		},
		"M": {
			S: aws.String(price.CollectionMethod),
		},
		"D": {
			S: aws.String(price.TransactionDateUTC),
		},
		"P": {
			N: aws.String(fmt.Sprintf("%d", price.Price)),
		},
	}, nil
}

func (p *FuelPrice) Unmarshal(record map[string]*dynamodb.AttributeValue) (err error) {
	fuelIdRecord, ok := record["FuelId"]
	if !ok {
		return nil
	}
	p.FuelID, err = strconv.Atoi(*fuelIdRecord.N)
	if err != nil {
		return err
	}

	methodRecord, ok := record["M"]
	if !ok {
		return nil
	}
	p.CollectionMethod = *methodRecord.S

	dateRecord, ok := record["D"]
	if !ok {
		return nil
	}
	p.TransactionDateUTC = *dateRecord.S

	priceRecord, ok := record["P"]
	if !ok {
		return nil
	}
	p.Price, err = strconv.Atoi(*priceRecord.N)
	if err != nil {
		return err
	}

	return nil
}

// FuelStation.Marshal returns a dynamodb representation of the FuelStation struct.
func (site FuelStation) Marshal() (map[string]*dynamodb.AttributeValue, error) {
	fuelIds := []*dynamodb.AttributeValue{}
	fuelTypes := map[string]*dynamodb.AttributeValue{}
	for fuelId, price := range site.FuelTypes {
		fuelIds = append(fuelIds, &dynamodb.AttributeValue{
			N: aws.String(fmt.Sprintf("%d", fuelId)),
		})

		marshalledPrice, err := price.Marshal()
		if err != nil {
			return nil, err
		}

		fuelTypes[fmt.Sprintf("%d", fuelId)] = &dynamodb.AttributeValue{
			M: marshalledPrice,
		}
	}

	item := map[string]*dynamodb.AttributeValue{
		"SiteId": {
			N: aws.String(fmt.Sprintf("%d", site.SiteID)),
		},
		"FuelIds": {
			L: fuelIds,
		},
		"FuelTypes": {
			M: fuelTypes,
		},
	}
	return item, nil
}

// FuelStation.Marshal returns a dynamodb representation of the FuelStation struct.
func (site *FuelStation) Unmarshal(record map[string]*dynamodb.AttributeValue) error {
	siteIdRecord, ok := record["SiteId"]
	if !ok {
		return nil
	}
	siteId, err := strconv.Atoi(*siteIdRecord.N)
	if err != nil {
		return err
	}
	site.SiteID = siteId

	fuelTypesRecord, ok := record["FuelTypes"]
	if !ok {
		return nil
	}
	site.FuelTypes = map[int]FuelPrice{}
	for fuelIdRecord, fuelRecord := range fuelTypesRecord.M {
		fuelId, err := strconv.Atoi(fuelIdRecord)
		if err != nil {
			return err
		}

		var fuelPrice FuelPrice
		err = fuelPrice.Unmarshal(fuelRecord.M)
		if err != nil {
			return err
		}

		site.FuelTypes[fuelId] = fuelPrice
	}

	return nil
}

// FuelPriceList.Marshal returns a dynamodb representation of the FuelPriceList struct.
func (prices FuelPriceList) Marshal() ([]map[string]*dynamodb.AttributeValue, error) {
	items := []map[string]*dynamodb.AttributeValue{}
	for _, site := range prices.Sites {
		item, err := site.Marshal()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (p *FuelPriceList) Unmarshal(records []map[string]*dynamodb.AttributeValue) error {
	p = &FuelPriceList{
		Sites: map[int]FuelStation{},
	}
	return nil
}
