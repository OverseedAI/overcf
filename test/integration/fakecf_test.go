package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
)

const (
	testToken = "test-token-abc123"
	testZoneID = "0023e105f4ecef8ad9ca31a8372d0c35"
	testZoneName = "example-test.com"
)

// fakeCF is an in-memory fake of the Cloudflare v4 REST API, covering the
// endpoints overcf uses: zone list/get and dns_records CRUD.
type fakeCF struct {
	server *httptest.Server

	mu       sync.Mutex
	records  map[string]map[string]any // record ID -> record object
	order    []string                  // record IDs in creation order
	nextID   int
	requests []string       // "METHOD /path" log for assertions
	lastBody map[string]any // body of the most recent POST/PATCH
}

func newFakeCF() *fakeCF {
	f := &fakeCF{
		records: make(map[string]map[string]any),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /zones", f.listZones)
	mux.HandleFunc("GET /zones/{zoneID}", f.getZone)
	mux.HandleFunc("GET /zones/{zoneID}/dns_records", f.listRecords)
	mux.HandleFunc("POST /zones/{zoneID}/dns_records", f.createRecord)
	mux.HandleFunc("GET /zones/{zoneID}/dns_records/{recordID}", f.getRecord)
	mux.HandleFunc("PATCH /zones/{zoneID}/dns_records/{recordID}", f.editRecord)
	mux.HandleFunc("DELETE /zones/{zoneID}/dns_records/{recordID}", f.deleteRecord)

	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		f.mu.Unlock()

		if r.Header.Get("Authorization") != "Bearer "+testToken {
			writeError(w, http.StatusForbidden, 9109, "Invalid access token")
			return
		}

		mux.ServeHTTP(w, r)
	}))

	return f
}

func (f *fakeCF) close() {
	f.server.Close()
}

func (f *fakeCF) zone() map[string]any {
	return map[string]any{
		"id":     testZoneID,
		"name":   testZoneName,
		"status": "active",
		"plan":   map[string]any{"name": "Free"},
		"name_servers": []string{
			"ana.ns.cloudflare.com",
			"bob.ns.cloudflare.com",
		},
	}
}

func (f *fakeCF) listZones(w http.ResponseWriter, r *http.Request) {
	results := []any{}

	name := r.URL.Query().Get("name")
	if page(r) == 1 && (name == "" || name == testZoneName) {
		results = append(results, f.zone())
	}

	writeList(w, results, page(r))
}

func (f *fakeCF) getZone(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("zoneID") != testZoneID {
		writeError(w, http.StatusNotFound, 7003, "Zone not found")
		return
	}
	writeResult(w, f.zone())
}

func (f *fakeCF) listRecords(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("zoneID") != testZoneID {
		writeError(w, http.StatusNotFound, 7003, "Zone not found")
		return
	}

	f.mu.Lock()
	results := []any{}
	if page(r) == 1 {
		for _, id := range f.order {
			results = append(results, f.records[id])
		}
	}
	f.mu.Unlock()

	writeList(w, results, page(r))
}

func (f *fakeCF) createRecord(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, 9100, "Malformed JSON in request body")
		return
	}

	f.mu.Lock()
	f.lastBody = body
	f.nextID++
	id := fmt.Sprintf("%032x", f.nextID)

	record := map[string]any{"id": id, "proxied": false, "ttl": float64(1)}
	for k, v := range body {
		record[k] = v
	}

	f.records[id] = record
	f.order = append(f.order, id)
	f.mu.Unlock()

	writeResult(w, record)
}

func (f *fakeCF) getRecord(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	record, ok := f.records[r.PathValue("recordID")]
	f.mu.Unlock()

	if !ok {
		writeError(w, http.StatusNotFound, 81044, "Record does not exist.")
		return
	}
	writeResult(w, record)
}

func (f *fakeCF) editRecord(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, 9100, "Malformed JSON in request body")
		return
	}

	f.mu.Lock()
	record, ok := f.records[r.PathValue("recordID")]
	if ok {
		f.lastBody = body
		for k, v := range body {
			record[k] = v
		}
	}
	f.mu.Unlock()

	if !ok {
		writeError(w, http.StatusNotFound, 81044, "Record does not exist.")
		return
	}
	writeResult(w, record)
}

func (f *fakeCF) deleteRecord(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("recordID")

	f.mu.Lock()
	_, ok := f.records[id]
	if ok {
		delete(f.records, id)
		for i, oid := range f.order {
			if oid == id {
				f.order = append(f.order[:i], f.order[i+1:]...)
				break
			}
		}
	}
	f.mu.Unlock()

	if !ok {
		writeError(w, http.StatusNotFound, 81044, "Record does not exist.")
		return
	}
	writeResult(w, map[string]any{"id": id})
}

// recordCount returns the number of stored DNS records.
func (f *fakeCF) recordCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.records)
}

// sawRequest reports whether a request matching "METHOD /path" was received.
func (f *fakeCF) sawRequest(methodAndPath string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, req := range f.requests {
		if req == methodAndPath {
			return true
		}
	}
	return false
}

func page(r *http.Request) int {
	p, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || p < 1 {
		return 1
	}
	return p
}

func writeResult(w http.ResponseWriter, result any) {
	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"errors":   []any{},
		"messages": []any{},
		"result":   result,
	})
}

func writeList(w http.ResponseWriter, results []any, page int) {
	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"errors":   []any{},
		"messages": []any{},
		"result":   results,
		"result_info": map[string]any{
			"page":        page,
			"per_page":    100,
			"count":       len(results),
			"total_count": len(results),
			"total_pages": 1,
		},
	})
}

func writeError(w http.ResponseWriter, status, code int, message string) {
	writeJSON(w, status, map[string]any{
		"success":  false,
		"errors":   []any{map[string]any{"code": code, "message": message}},
		"messages": []any{},
		"result":   nil,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
