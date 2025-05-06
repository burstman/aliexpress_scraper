package main

import (
	"encoding/json"
	"os"

	"github.com/chromedp/cdproto/network"
)

// LoadCookies loads cookies from a JSON file.
func LoadCookies(filename string) ([]*network.Cookie, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil, err
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var cookies []*network.Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, err
	}

	return cookies, nil
}

// SaveCookies saves cookies to a JSON file.
func SaveCookies(filename string, cookies []*network.Cookie) error {
	data, err := json.Marshal(cookies)
	if err != nil {
		return err
	}

	return WriteFile(filename, data, 0644)
}

// WriteFile writes data to a file with the specified permissions.
func WriteFile(filename string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filename, data, perm)
}
