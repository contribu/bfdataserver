package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/kkohtaka/go-bitflyer/pkg/api/realtime"
	//"github.com/kkohtaka/go-bitflyer/pkg/api/v1/board"
	"github.com/kkohtaka/go-bitflyer/pkg/api/v1/markets"
	"github.com/kkohtaka/go-bitflyer/pkg/api/v1/ticker"
	"github.com/urfave/cli/v2"
)

type Ticker struct {
	ProductCode string
	Timestamp time.Time
	BestBid float64
	BestAsk float64
	BestBidSize float64
	BestAskSize float64
}

type RawTicker struct {
	ProductCode string `json:"product_code"`
	Timestamp string `json:"timestamp"`
	BestBid float64 `json:"best_bid"`
	BestAsk float64 `json:"best_ask"`
	BestBidSize float64 `json:"best_bid_size"`
	BestAskSize float64 `json:"best_ask_size"`
}

// https://stackoverflow.com/questions/17156371/how-to-get-json-response-from-http-get
func getJson(client *http.Client, url string, target interface{}) error {
	r, err := client.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	return json.NewDecoder(r.Body).Decode(target)
}

func parseTimestamp(str string) (time.Time, error) {
	last := str[len(str) - 1:]
	normalized := str
	if last != "Z" {
		normalized += "Z"
	}
	return time.Parse(time.RFC3339Nano, normalized)
}

func getTicker(client *http.Client, ticker *Ticker) error {
	url := "https://api.bitflyer.com/v1/getticker?product_code=FX_BTC_JPY"
	raw := RawTicker{}
	if err := getJson(client, url, &raw); err != nil {
		return err
	}

	ticker.ProductCode = raw.ProductCode
	ticker.Timestamp, _ = parseTimestamp(raw.Timestamp)
	ticker.BestBid = raw.BestBid
	ticker.BestAsk = raw.BestAsk
	ticker.BestBidSize = raw.BestBidSize
	ticker.BestAskSize = raw.BestAskSize

	return nil
}

func initializeTicker(r *gin.Engine) {
	// 定期的にticker取得
	// アイデア
	// websocketと混ぜる
	// executionsやbookとも混ぜる

	latestTicker := Ticker{}
	latestTickers := map[string]Ticker{
		"http": Ticker{},
		"realtime": Ticker{},
	}
	delaySums := map[string]float64{
		"http": 0,
		"realtime": 0,
		"best": 0,
	}
	delayCount := 0
	tickerMtx := sync.Mutex{}

	updateTicker := func (t Ticker, tag string) {
		tickerMtx.Lock()
		defer tickerMtx.Unlock()

		if t.Timestamp.After(latestTickers[tag].Timestamp) {
			latestTickers[tag] = t
		}

		if t.Timestamp.After(latestTicker.Timestamp) {
			latestTicker = t
			log.Println(tag, t)
		}
	}

	{
		client := &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
					DualStack: true,
				}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          500, // ココ
				MaxIdleConnsPerHost:   100, // ココ
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
			Timeout: 60 * time.Second,
		}

		getTicker(client, &latestTicker)
		latestTickers["http"] = latestTicker
		latestTickers["realtime"] = latestTicker

		t := time.NewTicker(1000 * time.Millisecond)
		go func() {
			for {
				select {
				case <-t.C:
					ticker := Ticker{}
					if err := getTicker(client, &ticker); err != nil {
						log.Fatalln(err)
					} else {
						updateTicker(ticker, "http")
					}
				}
			}
		}()
	}

	{
		t := time.NewTicker(10 * time.Millisecond)
		go func() {
			for {
				select {
				case <-t.C:
					now := time.Now()
					for tag, ticker := range latestTickers {
						delay := now.Sub(ticker.Timestamp)
						delaySums[tag] += delay.Seconds()
					}
					delay := now.Sub(latestTicker.Timestamp)
					delaySums["best"] += delay.Seconds()
					delayCount += 1

					if delayCount % 500 == 0 {
						for tag, delaySum := range delaySums {
							log.Println(tag, delaySum / float64(delayCount))
						}
						delaySums = map[string]float64{
							"http": 0,
							"realtime": 0,
							"best": 0,
						}
						delayCount = 0
					}
				}
			}
		}()
	}

	{
		client := realtime.NewClient()
		session, err := client.Connect()
		if err != nil {
			log.Fatalln(err)
		}
		subscriber := realtime.NewSubscriber()

		subscriber.HandleTicker(
			[]markets.ProductCode{markets.ProductCode("FX_BTC_JPY")},
			func (resp ticker.Response) error {
				normalized := Ticker{}
				normalized.ProductCode = string(resp.ProductCode)
				normalized.Timestamp, _ = parseTimestamp(resp.Timestamp)
				normalized.BestBid = resp.BestBid
				normalized.BestAsk = resp.BestAsk
				normalized.BestBidSize = resp.BestBidSize
				normalized.BestAskSize = resp.BestAskSize
				updateTicker(normalized, "realtime")

				return nil
			})

		//asks := map[float64]float64{}
		//bids := map[float64]float64{}
		//
		//subscriber.HandleOrderBookUpdate(
		//	[]markets.ProductCode{markets.ProductCode("FX_BTC_JPY")},
		//	func (resp board.Response) error {
		//		for _, book := range resp.Asks {
		//			asks[book.Price] = book.Size
		//		}
		//		for _, book := range resp.Bids {
		//			bids[book.Price] = book.Size
		//		}
		//
		//		normalized := Ticker{}
		//		normalized.ProductCode = "FX_BTC_JPY"
		//
		//		normalized.BestAsk := 1e300
		//		for price, size := range asks {
		//			if normalized.BestAsk > price {
		//				normalized.BestAsk = price
		//				normalized.BestAskSize = size
		//			}
		//		}
		//
		//		normalized.BestBid := 0
		//		for price, size := range asks {
		//			if normalized.BestBid < price {
		//				normalized.BestBid = price
		//				normalized.BestBidSize = size
		//			}
		//		}
		//
		//		updateTicker(normalized)
		//
		//
		//		normalized.Timestamp, _ = parseTimestamp(resp.Timestamp)
		//
		//
		//		log.Println(normalized)
		//		return nil
		//	})

		if err = subscriber.ListenAndServe(session); err != nil {
			log.Fatalln(err)
		}
	}

	r.GET("/ticker", func(c *gin.Context) {
		c.JSON(200, latestTicker)
	})
}


func main() {
	app := &cli.App{
		Name: "bfdataserver",
		Usage: "fast access to bitflyer data",
		Action: func(c *cli.Context) error {
			r := gin.Default()
			initializeTicker(r)
			r.Run("localhost:8080")
			return nil
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
