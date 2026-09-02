package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/protocol/src"
)

func TestCatalogAppliesModelVariantWithoutOverwritingExplicitOptions(t *testing.T) {
	client := New("http://127.0.0.1:1", "")
	temperature := 0.2
	client.models.items = []protocol.Model{{ID: "model", Variants: map[string]protocol.ModelVariant{
		"precise": {Temperature: &temperature, Options: map[string]any{"reasoning": true, "effort": "high"}},
	}}}
	input := client.Catalog().ApplyVariant(protocol.ChatRequest{Model: "model", Variant: "precise", Options: map[string]any{"effort": "low"}})
	if input.Temperature == nil || *input.Temperature != temperature || input.Options["reasoning"] != true || input.Options["effort"] != "low" {
		t.Fatalf("input = %#v", input)
	}
	if _, err := client.Catalog().Find(context.Background(), "model"); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogRefreshesAndSortsModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []protocol.Model{{ID: "z-model"}, {ID: "a-model"}}})
	}))
	defer server.Close()
	client := New(server.URL, "")
	items, err := client.Catalog().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "a-model" || items[1].ID != "z-model" {
		t.Fatalf("models = %#v", items)
	}
	items[0].ID = "mutated"
	cached, ok := client.Catalog().FindCached("a-model")
	if !ok || cached.ID != "a-model" {
		t.Fatalf("cached = %#v, ok=%v", cached, ok)
	}
}
