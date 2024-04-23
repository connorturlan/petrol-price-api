package main

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

func TestSAFPISLISTConvertion(t *testing.T) {
	saPrices := SA_FuelPriceList{
		Prices: []SA_FuelPrice{
			{
				SiteId:             0,
				FuelId:             0,
				CollectionMethod:   "",
				TransactionDateUTC: "",
				Price:              9999,
			},
			{
				SiteId:             0,
				FuelId:             1,
				CollectionMethod:   "",
				TransactionDateUTC: "",
				Price:              9999,
			},
			{
				SiteId:             1,
				FuelId:             0,
				CollectionMethod:   "",
				TransactionDateUTC: "",
				Price:              9999,
			},
		},
	}

	intPrices, err := saPrices.ToPriceList()
	if err != nil {
		t.Error(err)
	}

	if len(intPrices.Sites) != 2 {
		t.Errorf("expected 2 sites, got %d", len(intPrices.Sites))
	}

	if len(intPrices.Sites[0].FuelTypes) != 2 {
		t.Errorf("expected 2 fuel types, got %d", len(intPrices.Sites[0].FuelTypes))
	}

	if len(intPrices.Sites[1].FuelTypes) != 1 {
		t.Errorf("expected 1 fuel type, got %d", len(intPrices.Sites[1].FuelTypes))
	}
}

func TestFuelPriceMarshalling(t *testing.T) {
	intPrice := FuelPrice{
		FuelID:             0,
		CollectionMethod:   "1",
		TransactionDateUTC: "2",
		Price:              3,
	}

	dbPrice, err := intPrice.Marshal()
	if err != nil {
		t.Error(err)
	}

	v, ok := dbPrice["FuelId"]
	if !ok {
		t.Error("key, FuelId is missing")
	}
	if *v.N != "0" {
		t.Errorf("key, FuelId returned unexpected value: %s", *v.N)
	}

	v, ok = dbPrice["M"]
	if !ok {
		t.Error("key, M is missing")
	}
	if *v.S != "1" {
		t.Errorf("key, M returned unexpected value: %s", *v.N)
	}

	v, ok = dbPrice["D"]
	if !ok {
		t.Error("key, D is missing")
	}
	if *v.S != "2" {
		t.Errorf("key, D returned unexpected value: %s", *v.N)
	}

	v, ok = dbPrice["P"]
	if !ok {
		t.Error("key, P is missing")
	}
	if *v.N != "3" {
		t.Errorf("key, P returned unexpected value: %s", *v.N)
	}
}

func TestFuelStationMarshalling(t *testing.T) {
	intSite := FuelStation{
		SiteID: 0,
		FuelTypes: map[int]FuelPrice{
			0: {
				FuelID:             0,
				CollectionMethod:   "1",
				TransactionDateUTC: "2",
				Price:              3,
			},
			1: {
				FuelID:             1,
				CollectionMethod:   "1",
				TransactionDateUTC: "2",
				Price:              3,
			},
		},
	}

	dbSite, err := intSite.Marshal()
	if err != nil {
		t.Error(err)
	}

	v, ok := dbSite["SiteId"]
	if !ok {
		t.Error("key, SiteId is missing")
	}
	if *v.N != "0" {
		t.Errorf("key, SiteId returned unexpected value: %s", *v.N)
	}

	fuelMap, ok := dbSite["FuelTypes"]
	if !ok {
		t.Error("key, FuelTypes is missing")
	}
	if len(fuelMap.M) != 2 {
		t.Errorf("expected FuelTypes to have length 2, got %d", len(fuelMap.M))
	}
}

func TestFuelListMarshalling(t *testing.T) {
	intList := FuelPriceList{
		Sites: map[int]FuelStation{
			0: {
				SiteID: 0,
				FuelTypes: map[int]FuelPrice{
					0: {
						FuelID:             0,
						CollectionMethod:   "1",
						TransactionDateUTC: "2",
						Price:              3,
					},
					1: {
						FuelID:             1,
						CollectionMethod:   "1",
						TransactionDateUTC: "2",
						Price:              3,
					},
				},
			},
			1: {
				SiteID: 1,
				FuelTypes: map[int]FuelPrice{
					2: {
						FuelID:             2,
						CollectionMethod:   "1",
						TransactionDateUTC: "2",
						Price:              3,
					},
				},
			},
		},
	}

	dbList, err := intList.Marshal()
	if err != nil {
		t.Error(err)
	}

	if len(dbList) != 2 {
		t.Errorf("expected 2 entries, got %d", len(dbList))
	}

	v, ok := dbList[0]["SiteId"]
	if !ok {
		t.Error("key, SiteId is missing")
	}
	if *v.N != "0" {
		t.Errorf("key, SiteId returned unexpected value: %s", *v.N)
	}

	v, ok = dbList[0]["FuelTypes"]
	if !ok {
		t.Error("key, FuelTypes is missing")
	}
	vM := v.M
	if len(vM) != 2 {
		t.Errorf("expected length of FuelTypes to be 2, got %d", len(vM))
	}

	v, ok = dbList[1]["SiteId"]
	if !ok {
		t.Error("key, SiteId is missing")
	}
	if *v.N != "1" {
		t.Errorf("key, SiteId returned unexpected value: %s", *v.N)
	}

	v, ok = dbList[1]["FuelTypes"]
	if !ok {
		t.Error("key, FuelTypes is missing")
	}
	vM = v.M
	if len(vM) != 1 {
		t.Errorf("expected length of FuelTypes to be 1, got %d", len(vM))
	}
}

func TestFuelPriceUnmarshalling(t *testing.T) {
	dbPrice := map[string]*dynamodb.AttributeValue{
		"FuelId": {
			N: aws.String(fmt.Sprintf("%d", 1)),
		},
		"M": {
			S: aws.String("2"),
		},
		"D": {
			S: aws.String("3"),
		},
		"P": {
			N: aws.String(fmt.Sprintf("%d", 4444)),
		},
	}

	var intPrice FuelPrice
	err := intPrice.Unmarshal(dbPrice)
	if err != nil {
		t.Error(err)
	}

	if intPrice.FuelID != 1 {
		t.Errorf("expected FuelId == 1, got %d", intPrice.FuelID)
	}
	if intPrice.CollectionMethod != "2" {
		t.Errorf("expected CollectionMethod == '2', got %s", intPrice.CollectionMethod)
	}
	if intPrice.TransactionDateUTC != "3" {
		t.Errorf("expected TransactionDateUTC == '2', got %s", intPrice.TransactionDateUTC)
	}
	if intPrice.Price != 4444 {
		t.Errorf("expected FuelId == 4444, got %d", intPrice.Price)
	}

}

// func TestFuelStationMarshalling(t *testing.T) {
// 	intSite := FuelStation{
// 		SiteID: 0,
// 		FuelTypes: map[int]FuelPrice{
// 			0: {
// 				FuelID:             0,
// 				CollectionMethod:   "1",
// 				TransactionDateUTC: "2",
// 				Price:              3,
// 			},
// 			1: {
// 				FuelID:             1,
// 				CollectionMethod:   "1",
// 				TransactionDateUTC: "2",
// 				Price:              3,
// 			},
// 		},
// 	}

// 	dbSite, err := intSite.Marshal()
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	v, ok := dbSite["SiteId"]
// 	if !ok {
// 		t.Error("key, SiteId is missing")
// 	}
// 	if *v.N != "0" {
// 		t.Errorf("key, SiteId returned unexpected value: %s", *v.N)
// 	}

// 	fuelMap, ok := dbSite["FuelTypes"]
// 	if !ok {
// 		t.Error("key, FuelTypes is missing")
// 	}
// 	if len(fuelMap.M) != 2 {
// 		t.Errorf("expected FuelTypes to have length 2, got %d", len(fuelMap.M))
// 	}
// }

// func TestFuelListMarshalling(t *testing.T) {

// 	intList := FuelPriceList{
// 		Sites: map[int]FuelStation{
// 			0: {
// 				SiteID: 0,
// 				FuelTypes: map[int]FuelPrice{
// 					0: {
// 						FuelID:             0,
// 						CollectionMethod:   "1",
// 						TransactionDateUTC: "2",
// 						Price:              3,
// 					},
// 					1: {
// 						FuelID:             1,
// 						CollectionMethod:   "1",
// 						TransactionDateUTC: "2",
// 						Price:              3,
// 					},
// 				},
// 			},
// 			1: {
// 				SiteID: 1,
// 				FuelTypes: map[int]FuelPrice{
// 					2: {
// 						FuelID:             2,
// 						CollectionMethod:   "1",
// 						TransactionDateUTC: "2",
// 						Price:              3,
// 					},
// 				},
// 			},
// 		},
// 	}

// 	dbList, err := intList.Marshal()
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	if len(dbList) != 2 {
// 		t.Errorf("expected 2 entries, got %d", len(dbList))
// 	}

// 	v, ok := dbList[0]["SiteId"]
// 	if !ok {
// 		t.Error("key, SiteId is missing")
// 	}
// 	if *v.N != "0" {
// 		t.Errorf("key, SiteId returned unexpected value: %s", *v.N)
// 	}

// 	v, ok = dbList[0]["FuelTypes"]
// 	if !ok {
// 		t.Error("key, FuelTypes is missing")
// 	}
// 	vM := v.M
// 	if len(vM) != 2 {
// 		t.Errorf("expected length of FuelTypes to be 2, got %d", len(vM))
// 	}

// 	v, ok = dbList[1]["SiteId"]
// 	if !ok {
// 		t.Error("key, SiteId is missing")
// 	}
// 	if *v.N != "1" {
// 		t.Errorf("key, SiteId returned unexpected value: %s", *v.N)
// 	}

// 	v, ok = dbList[1]["FuelTypes"]
// 	if !ok {
// 		t.Error("key, FuelTypes is missing")
// 	}
// 	vM = v.M
// 	if len(vM) != 1 {
// 		t.Errorf("expected length of FuelTypes to be 1, got %d", len(vM))
// 	}
// }
