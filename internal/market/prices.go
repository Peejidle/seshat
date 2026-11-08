package market

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

const apiURL = "https://api.warframe.market/"

// PricesFile is the cache of prices looked up, next to the item list.
const PricesFile = "prices.json"

// PriceTTL is how long a looked-up price is trusted. Who's online selling
// changes by the hour, so a day would be too long for "list it at".
const PriceTTL = 6 * time.Hour

// gap spaces out requests: the market allows about three a second.
const gap = 350 * time.Millisecond

// Price is what one rank of an item goes for.
type Price struct {
	// Median is the typical price paid over the last 48 hours, or if
	// nothing sold then, on the last day anything did in 90. 0 if never.
	Median int `json:"median"`
	// Sold is how many sold in the last 48 hours. Only sales people
	// report on the site count, so the true number is higher.
	Sold   int `json:"sold"`
	Lowest int `json:"lowest"` // the cheapest seller online now; 0 if none
	TopBuy int `json:"topBuy"` // the best offer from a buyer online now; 0 if none
}

// Prices is everything looked up for one item.
type Prices struct {
	Slug    string        `json:"slug"`
	Fetched time.Time     `json:"fetched"`
	Ranks   map[int]Price `json:"ranks"` // by rank; unranked items use 0
}

// Client asks warframe.market for prices, no faster than it allows.
type Client struct {
	http      *http.Client
	userAgent string
	base      string        // the API's address; a test server's in tests
	gap       time.Duration // the least time between requests
	mu        sync.Mutex
	next      time.Time // when the next request may go
}

func NewClient(userAgent string) *Client {
	return &Client{
		http: &http.Client{Timeout: 30 * time.Second}, userAgent: userAgent,
		base: apiURL, gap: gap,
	}
}

// errBusy is a 429: the market wants us to slow down.
var errBusy = errors.New("warframe.market says to slow down")

// get fetches one API path into v, waiting its turn first. A 429 gets one
// retry after a pause.
func (c *Client) get(path string, v any) error {
	err := c.getOnce(path, v)
	if errors.Is(err, errBusy) {
		time.Sleep(2 * time.Second)
		err = c.getOnce(path, v)
	}
	return err
}

func (c *Client) getOnce(path string, v any) error {
	c.mu.Lock()
	wait := time.Until(c.next)
	c.next = time.Now().Add(max(wait, 0) + c.gap)
	c.mu.Unlock()
	time.Sleep(wait) // returns at once if wait <= 0

	req, _ := http.NewRequest("GET", c.base+path, nil)
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return json.NewDecoder(resp.Body).Decode(v)
	case http.StatusTooManyRequests:
		return errBusy
	default:
		return fmt.Errorf("%s: %s", path, resp.Status)
	}
}

// statBucket is one hour (or day) of reported sales at one rank.
type statBucket struct {
	Volume int     `json:"volume"`
	Median float64 `json:"median"`
	Rank   int     `json:"mod_rank"` // absent, so 0, for unranked items
}

type order struct {
	Platinum int `json:"platinum"`
	Rank     int `json:"rank"`
}

// Fetch looks up an item's prices: rank 0 and max rank for an arcane, the
// one price for anything else. The sales history comes from the old v1
// API, the only one that has it.
func (c *Client) Fetch(it Item) (Prices, error) {
	var stats struct {
		Payload struct {
			Closed struct {
				Hours []statBucket `json:"48hours"`
				Days  []statBucket `json:"90days"`
			} `json:"statistics_closed"`
		} `json:"payload"`
	}
	slug := url.PathEscape(it.Slug)
	if err := c.get("v1/items/"+slug+"/statistics", &stats); err != nil {
		return Prices{}, err
	}
	p := Prices{Slug: it.Slug, Fetched: time.Now(), Ranks: map[int]Price{}}
	ranks := []int{0}
	if it.MaxRank > 0 {
		ranks = append(ranks, it.MaxRank)
	}
	for _, r := range ranks {
		pr := summarise(stats.Payload.Closed.Hours, stats.Payload.Closed.Days, r)

		// The five cheapest sellers and best buyers online, at this rank.
		var top struct {
			Data struct {
				Sell []order `json:"sell"`
				Buy  []order `json:"buy"`
			} `json:"data"`
		}
		path := "v2/orders/item/" + slug + "/top"
		if it.MaxRank > 0 {
			path += "?rank=" + strconv.Itoa(r)
		}
		if err := c.get(path, &top); err != nil {
			return Prices{}, err
		}
		if len(top.Data.Sell) > 0 {
			pr.Lowest = top.Data.Sell[0].Platinum // cheapest first
		}
		if len(top.Data.Buy) > 0 {
			pr.TopBuy = top.Data.Buy[0].Platinum // highest first
		}
		p.Ranks[r] = pr
	}
	return p, nil
}

// summarise turns the sales history at one rank into a typical price and
// how many sold. The typical price is the median of each hour's median,
// weighted by how many sold that hour, so one odd sale can't swing it.
func summarise(hours, days []statBucket, rank int) Price {
	var bs []statBucket
	sold := 0
	for _, b := range hours {
		if b.Rank == rank && b.Volume > 0 {
			bs = append(bs, b)
			sold += b.Volume
		}
	}
	if sold == 0 {
		// Nothing in 48 hours: fall back on the latest day that had sales.
		for i := len(days) - 1; i >= 0; i-- {
			if days[i].Rank == rank && days[i].Volume > 0 {
				return Price{Median: int(days[i].Median + 0.5)}
			}
		}
		return Price{}
	}
	sort.Slice(bs, func(i, j int) bool { return bs[i].Median < bs[j].Median })
	half, seen := (sold+1)/2, 0
	for _, b := range bs {
		seen += b.Volume
		if seen >= half {
			return Price{Median: int(b.Median + 0.5), Sold: sold}
		}
	}
	return Price{Sold: sold} // not reached
}

// PriceCache keeps looked-up prices between runs, by slug.
type PriceCache struct {
	path    string
	Entries map[string]Prices
}

// LoadPrices reads the cache; a missing or broken one starts empty, since
// it only saves lookups.
func LoadPrices(path string) *PriceCache {
	pc := &PriceCache{path: path, Entries: map[string]Prices{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &pc.Entries)
	}
	if pc.Entries == nil {
		pc.Entries = map[string]Prices{}
	}
	return pc
}

// Get returns an item's cached prices and whether they're still fresh.
func (pc *PriceCache) Get(slug string) (Prices, bool) {
	p, ok := pc.Entries[slug]
	return p, ok && time.Since(p.Fetched) < PriceTTL
}

func (pc *PriceCache) Put(p Prices) { pc.Entries[p.Slug] = p }

func (pc *PriceCache) Save() error {
	b, err := json.Marshal(pc.Entries)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(pc.path), 0o755); err != nil {
		return err
	}
	tmp := pc.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, pc.path)
}
