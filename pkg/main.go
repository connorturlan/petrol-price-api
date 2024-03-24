package petrolapi

type SA_FuelPriceList struct {
	Prices []SA_FuelPrice `json:"SitePrices"`
}

type SA_FuelPrice struct {
	SiteId             int     `json:"SiteId"`
	FuelId             int     `json:"FuelId"`
	CollectionMethod   string  `json:"CollectionMethod"`
	TransactionDateUTC string  `json:"TransactionDateUTC"`
	Price              float64 `json:"Price"`
}

type FuelPriceList struct {
	Prices map[int]FuelStation `json:"SitePrices"`
}

type FuelStation struct {
	SiteID    int               `json:"SiteId"`
	FuelTypes map[int]FuelPrice `json:"FuelTypes"`
}

type FuelPrice struct {
	FuelID             int     `json:"FuelId"`
	CollectionMethod   string  `json:"CollectionMethod"`
	TransactionDateUTC string  `json:"TransactionDateUTC"`
	Price              float64 `json:"Price"`
}

type PetrolStationList struct {
	Sites []PetrolStationSite `json:"S"`
}

type PetrolStationSite struct {
	SiteID        int     `json:"S"`
	Address       string  `json:"A"`
	Name          string  `json:"N"`
	BrandID       int     `json:"B"`
	Postcode      string  `json:"P"`
	GooglePlaceID string  `json:"GPI"`
	Latitude      float64 `json:"Lat"`
	Longitude     float64 `json:"Lng"`
}
