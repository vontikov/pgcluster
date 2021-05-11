package pg

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func Handlers(c Cluster) map[string]func(http.ResponseWriter, *http.Request) {
	return map[string]func(http.ResponseWriter, *http.Request){
		"/pg/version":    versionHandler(c),
		"/pg/alive":      aliveHandler(c),
		"/pg/inrecovery": inrecoveryHandler(c),
		"/pg/masterinfo": masterinfoHandler(c),
		"/pg/stop":       stopHandler(c),
		"/pg/start":      startHandler(c),
		"/pg/promote":    promoteHandler(c),
		"/pg/backup":     backupHandler(c),
	}
}

func versionHandler(c Cluster) func(http.ResponseWriter, *http.Request) {
	return getFunc(func() ([]byte, error) {
		major, minor, err := c.Version()
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf(`"{"major":%d,"minor":%d}`, major, minor)), nil
	})
}

func aliveHandler(c Cluster) func(http.ResponseWriter, *http.Request) {
	return getFunc(func() ([]byte, error) {
		alive, err := c.Alive()
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf(`"{"alive":%v}`, alive)), nil
	})
}

func inrecoveryHandler(c Cluster) func(http.ResponseWriter, *http.Request) {
	return getFunc(func() ([]byte, error) {
		inrecovery, err := c.InRecovery()
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf(`"{"inrecovery":%v}`, inrecovery)), nil
	})
}

func masterinfoHandler(c Cluster) func(http.ResponseWriter, *http.Request) {
	return getFunc(func() ([]byte, error) {
		mi, err := c.MasterInfo()
		if err != nil {
			return nil, err
		}
		b, err := json.Marshal(mi)
		if err != nil {
			return nil, err
		}
		return b, nil
	})
}

func stopHandler(c Cluster) func(http.ResponseWriter, *http.Request) {
	return postFunc(c.Stop)
}

func startHandler(c Cluster) func(http.ResponseWriter, *http.Request) {
	return postFunc(c.Start)
}

func promoteHandler(c Cluster) func(http.ResponseWriter, *http.Request) {
	return postFunc(c.Promote)
}

func backupHandler(c Cluster) func(http.ResponseWriter, *http.Request) {
	return func(http.ResponseWriter, *http.Request) {
		// TODO
		panic("backupHandler() not implemented yet")
	}
}

func getFunc(f func() ([]byte, error)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte(http.StatusText(http.StatusMethodNotAllowed)))
			return
		}

		b, err := f()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf("%s: %s", http.StatusText(http.StatusInternalServerError), err.Error())))
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
	}
}

func postFunc(f func() error) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte(http.StatusText(http.StatusMethodNotAllowed)))
			return
		}

		if err := f(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf("%s: %s", http.StatusText(http.StatusInternalServerError), err.Error())))
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
