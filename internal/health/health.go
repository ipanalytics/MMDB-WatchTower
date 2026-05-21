package health

import (
	"encoding/json"
	"net/http"

	"mmdb-watchtower/internal/config"
	"mmdb-watchtower/internal/state"
)

func Serve(addr string, cfg config.Config, channel string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		report := struct {
			Ready     bool              `json:"ready"`
			Databases map[string]string `json:"databases"`
		}{Ready: true, Databases: map[string]string{}}
		for _, raw := range cfg.Databases {
			db, err := config.ResolveDatabase(raw, channel)
			if err != nil {
				report.Ready = false
				report.Databases[raw.Name] = err.Error()
				continue
			}
			st, err := state.LoadForDB(db)
			if err != nil {
				report.Ready = false
				report.Databases[db.Name] = err.Error()
				continue
			}
			current := st.DatabaseState(db.Name)
			if current.Status != "healthy" {
				report.Ready = false
				if current.Status == "" {
					report.Databases[db.Name] = "unknown"
				} else {
					report.Databases[db.Name] = current.Status
				}
				continue
			}
			report.Databases[db.Name] = "healthy"
		}
		if !report.Ready {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(report)
	})
	return http.ListenAndServe(addr, mux)
}
