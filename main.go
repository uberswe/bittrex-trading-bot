package main

//MIT License
//
//Copyright (c) 2018 Markus Tenghamn
//
//Permission is hereby granted, free of charge, to any person obtaining a copy
//of this software and associated documentation files (the "Software"), to deal
//in the Software without restriction, including without limitation the rights
//to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
//copies of the Software, and to permit persons to whom the Software is
//furnished to do so, subject to the following conditions:
//
//The above copyright notice and this permission notice shall be included in all
//copies or substantial portions of the Software.
//
//THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
//IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
//FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
//AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
//LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
//OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
//SOFTWARE.

import (
	"log"
	"github.com/OpinionatedGeek/go-bittrex"
	"time"
	"github.com/shopspring/decimal"
)

const (
	API_KEY       = ""
	API_SECRET    = ""
	BUY_STRING    = "BTC"
	SELL_STRING   = "VTC"
	MARKET_STRING = BUY_STRING + "-" + SELL_STRING
	MIN_GAIN      = 0.02
	MAX_LOSS      = 0.02
	ORDER_RANGE	  = 0.02

	BUY_TRIGGER    = 10000.0
	SELL_TRIGGER   = -10000.0
	ORDER_VARIANCE = 0.02
)

var (
	balances     []bittrex.Balance
	orders       []bittrex.Order
	ticker       = bittrex.Ticker{}
	lastPrice    float64
	lastBuyPrice = 0.00
	buySellIndex = 0.00
	openOrder    = false
	readyToRun   = false

	highIndex = 0.00
	lowIndex  = 0.00
)

func main() {

	// Bittrex client
	bittrexClient := bittrex.New(API_KEY, API_SECRET)

	go updateStats(bittrexClient)

	// A Simple Trading Strategy
	// We create our own buy/sell index, this resets with every buy and sell
	// If we buy and incur a 2% loss we sell at current ask
	// If we buy and make at least a 2% profit and our index is sell we sell
	// If we place an order and it does not clear and market moves -+2% we cancel
	// Every trade has a 0.25% fee

	ch := make(chan bittrex.ExchangeState, 16)

	go subscribeMarket(bittrexClient, ch)

	for st := range ch {
		// Order placed
		for _, b := range st.Buys {
			//log.Println("Buy: ", b.Quantity, " for ", b.Rate, " as ", b.Type)
			quantity, _ := b.Quantity.Float64()
			rate, _ := b.Rate.Float64()
			calculateIndex(true, quantity, rate)
		}
		for _, s := range st.Sells {
			//log.Println("Sell: ", s.Quantity, " for ", s.Rate, " as ", s.Type)
			quantity, _ := s.Quantity.Float64()
			rate, _ := s.Rate.Float64()
			calculateIndex(false, quantity, rate)
		}
		// Order actually fills
		for _, f := range st.Fills {
			//log.Println("Fill: ", f.Quantity, " for ", f.Rate, " as ", f.OrderType)
			// We could say that lastPrice is technically the fill price
			lastPrice, _ = f.Rate.Float64()
		}
		log.Printf("BuySellIndex: 		%.4f\n", buySellIndex)
		decideBuySell(bittrexClient)
	}
}

func subscribeMarket(b *bittrex.Bittrex, ch chan bittrex.ExchangeState) {
		log.Println("Connecting to:", MARKET_STRING)
		err := b.SubscribeExchangeUpdate(MARKET_STRING, ch, nil)
		if err != nil {
			log.Println("Error:", err)
		}
		log.Println("Reconnecting....")
		go subscribeMarket(b, ch)
}

func decideBuySell(b *bittrex.Bittrex) {
	if openOrder {
		// Should we close the open order?
		for _, o := range orders {
			ppu, _ := o.PricePerUnit.Float64()
			log.Printf("Order percent: %.4f\n", ppu/lastPrice)
			if ppu/lastPrice > (1.00+ORDER_VARIANCE) || ppu/lastPrice < (1.00-ORDER_VARIANCE) {
				log.Println("Canceled order: ", o.OrderUuid)
				b.CancelOrder(o.OrderUuid)
				// We assume we only have one order at a time
			}
		}
	}
	// If we have no open order should we buy or sell?
	if !openOrder {
		if buySellIndex > BUY_TRIGGER {
			log.Println("BUY TRIGGER ACTIVE!")
			for _, bals := range balances {
				bal, _ := bals.Balance.Float64()
				if BUY_STRING == bals.Currency {
					//log.Printf("Bal: %.4f %s == %s\n", bal/lastPrice, SELL_STRING, bals.Currency)
				}
				if bal > 0.01 && BUY_STRING == bals.Currency && lastPrice > 0.00 {
					// Place buy order
					log.Printf("Placed buy order of %.4f %s at %.8f\n=================================================\n", (bal/lastPrice)-5, BUY_STRING, lastPrice)
					order, err := b.BuyLimit(MARKET_STRING, decimal.NewFromFloat((bal/lastPrice)-5), decimal.NewFromFloat(lastPrice))
					if err != nil {
						log.Println("ERROR ", err)
					} else {
						log.Println("Confirmed: ", order)
					}
					lastBuyPrice = lastPrice
					openOrder = true
				}
			}
		} else if buySellIndex < SELL_TRIGGER {
			log.Println("SELL TRIGGER ACTIVE!")
			for _, bals := range balances {
				bal, _ := bals.Balance.Float64()
				if SELL_STRING == bals.Currency {
					//allow := "false"
					//if allowSell() {
					//	allow = "true"
					//}
					//log.Printf("Bal: %.4f %s == %s && %s\n", bal, BUY_STRING, bals.Currency, allow)
				}
				if bal > 0.01 && SELL_STRING == bals.Currency && lastPrice > 0.00 && allowSell() {
					// Place sell order
					log.Printf("Placed sell order of %.4f %s at %.8f\n=================================================\n", bal, BUY_STRING, lastPrice)
					order, err := b.SellLimit(MARKET_STRING, decimal.NewFromFloat(bal), decimal.NewFromFloat(lastPrice))
					if err != nil {
						log.Println("ERROR ", err)
					} else {
						log.Println("Confirmed: ", order)
					}
					openOrder = true
				}
			}
		}
	}
}

func allowSell() bool {
	if lastBuyPrice > 0 {
		gain := lastPrice / lastBuyPrice
		if gain < (1.00 - MAX_LOSS) {
			return true
		}
		if gain < (1.00 + MIN_GAIN) {
			return false
		}
	}
	return true
}

func calculateIndex(buy bool, q float64, r float64) {
	// q is quantity VTC
	// r is the rate
	percent := 0.00
	// Calculate percentage of rate
	if r > 0 && q > 0 && lastPrice > 0 && readyToRun {
		percent = lastPrice / r
		if buy {
			//log.Printf("Buy percent: %.4f\n", percent)
			//log.Printf("Buy quantity: %.4f\n", q)
			if percent > (1.00 - ORDER_RANGE) && percent < (1.00 + ORDER_RANGE) {
				buySellIndex = buySellIndex + (percent * q)
			}
		} else {
			//log.Printf("Sell percent: %.4f\n", percent)
			//log.Printf("Sell quantity: %.4f\n", q)
			if percent > (1.00 - ORDER_RANGE) && percent < (1.00 + ORDER_RANGE) {
				percent = percent - 2.00 // Reverse percent, lower is higher
				buySellIndex = buySellIndex + (percent * q)
			}
		}
	}
	if buySellIndex > highIndex {
		highIndex = buySellIndex
	}
	if buySellIndex < lowIndex {
		lowIndex = buySellIndex
	}
	// Reset really high or low numbers due to startup
	if highIndex > 5000000.00 || lowIndex < -5000000.00 {
		highIndex = 0.00
		lowIndex = 0.00
		buySellIndex = 0.00
	}
}

func updateStats(b *bittrex.Bittrex) {
	var err error = nil
	for {
		go func(b *bittrex.Bittrex) {
			balances, err = b.GetBalances()
			orders, err = b.GetOpenOrders(MARKET_STRING)
			ticker, err = b.GetTicker(MARKET_STRING)
			if err != nil {
				log.Println("Error:", err)
				// Pause calculations in case of error
				readyToRun = false
			}

			log.Printf("====================================\n")
			log.Printf("Last price: 		%v\n", ticker.Last)
			log.Printf("Index: 			%.4f\n", buySellIndex)
			log.Printf("High Index: 		%.4f\n", highIndex)
			log.Printf("Low Index: 			%.4f\n", lowIndex)
			lastPrice, _ = ticker.Last.Float64()
			buySellIndex = 0.00

			log.Printf("Bid:			%v\n", ticker.Bid)
			log.Printf("Ask:			%v\n", ticker.Ask)

			// Do we have an open order?
			openOrder = len(orders) > 0

			for _, o := range orders {
				log.Println("Pending order: ", o.OrderType, " Quanitity: ", o.QuantityRemaining, "/", o.Quantity, " Price: ", o.PricePerUnit)
			}

			// Where do we have balances
			for _, b := range balances {
				bal, _ := b.Balance.Float64()
				if bal > 0.00 {
					log.Printf("%s:			%v %s %v\n", b.Currency, b.Available, "/", b.Balance)
				}
			}
			log.Printf("====================================\n")

		}(b)
		<-time.After(60 * time.Second)
		// Wait 60 to init and collect data
		readyToRun = true
	}
}
