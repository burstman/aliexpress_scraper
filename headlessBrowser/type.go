package main

// Product represents an AliExpress product with its details.
type Product struct {
	Name   string   `json:"name"`
	Price  string   `json:"price"`
	Orders int      `json:"orders"`
	Rating *float64 `json:"rating"`
	Link   string   `json:"link"`
}
