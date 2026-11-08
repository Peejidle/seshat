package market

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestSummarise(t *testing.T) {
	hours := []statBucket{
		{Volume: 10, Median: 120, Rank: 5},
		{Volume: 1, Median: 300, Rank: 5}, // one odd sale
		{Volume: 4, Median: 110, Rank: 5},
		{Volume: 50, Median: 7, Rank: 0},
	}
	if p := summarise(hours, nil, 5); p.Median != 120 || p.Sold != 15 {
		t.Errorf("rank 5 = %+v, want median 120 from 15 sold", p)
	}
	if p := summarise(hours, nil, 0); p.Median != 7 || p.Sold != 50 {
		t.Errorf("rank 0 = %+v", p)
	}
	// Nothing sold in 48 hours: the latest day with sales stands in.
	days := []statBucket{{Volume: 3, Median: 40, Rank: 3}, {Volume: 2, Median: 44.6, Rank: 3}}
	if p := summarise(nil, days, 3); p.Median != 45 || p.Sold != 0 {
		t.Errorf("fallback = %+v, want median 45 and no recent sales", p)
	}
	if p := summarise(nil, nil, 3); p != (Price{}) {
		t.Errorf("never sold = %+v", p)
	}
}

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path + "?" + r.URL.RawQuery {
		case "/v1/items/arcane_energize/statistics?":
			w.Write([]byte(`{"payload":{"statistics_closed":{"48hours":[
				{"volume":3,"median":120,"mod_rank":5},{"volume":9,"median":7,"mod_rank":0}],"90days":[]}}}`))
		case "/v2/orders/item/arcane_energize/top?rank=0":
			w.Write([]byte(`{"data":{"sell":[{"platinum":6,"rank":0},{"platinum":7,"rank":0}],"buy":[{"platinum":5,"rank":0}]}}`))
		case "/v2/orders/item/arcane_energize/top?rank=5":
			w.Write([]byte(`{"data":{"sell":[{"platinum":119,"rank":5}],"buy":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := NewClient("test")
	c.base, c.gap = srv.URL+"/", 0

	p, err := c.Fetch(Item{Slug: "arcane_energize", MaxRank: 5})
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]Price{0: {Median: 7, Sold: 9, Lowest: 6, TopBuy: 5}, 5: {Median: 120, Sold: 3, Lowest: 119}}
	for r, w := range want {
		if p.Ranks[r] != w {
			t.Errorf("rank %d = %+v, want %+v", r, p.Ranks[r], w)
		}
	}
}

func TestPriceCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prices.json")
	pc := LoadPrices(path)
	pc.Put(Prices{Slug: "new", Fetched: time.Now(), Ranks: map[int]Price{0: {Median: 3}}})
	pc.Put(Prices{Slug: "old", Fetched: time.Now().Add(-PriceTTL - time.Minute)})
	if err := pc.Save(); err != nil {
		t.Fatal(err)
	}
	pc = LoadPrices(path)
	if p, fresh := pc.Get("new"); !fresh || p.Ranks[0].Median != 3 {
		t.Errorf("new = %+v, fresh %v", p, fresh)
	}
	if _, fresh := pc.Get("old"); fresh {
		t.Error("a price older than PriceTTL should be stale")
	}
}
