// Copyright 2018 The conprof Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"net/http"

	"github.com/conprof/conprof/api"
	"github.com/conprof/conprof/pprofui"
	"github.com/conprof/conprof/web"
	"github.com/conprof/db/tsdb"
	"github.com/conprof/db/tsdb/wal"
	"github.com/go-kit/kit/log"
	"github.com/julienschmidt/httprouter"
	"github.com/oklog/run"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

// registerSampler registers a sampler command.
func registerWeb(m map[string]setupFunc, app *kingpin.Application, name string, reloadCh chan struct{}) {
	cmd := app.Command(name, "Run a web interface to view profiles from a storage.")

	storagePath := cmd.Flag("storage.tsdb.path", "Directory to read storage from.").
		Default("./data").String()
	retention := modelDuration(cmd.Flag("storage.tsdb.retention.time", "How long to retain raw samples on local storage. 0d - disables this retention").Default("15d"))

	m[name] = func(g *run.Group, mux *http.ServeMux, logger log.Logger, reg *prometheus.Registry, tracer opentracing.Tracer, debugLogging bool) error {
		db, err := tsdb.Open(
			*storagePath,
			logger,
			prometheus.DefaultRegisterer,
			&tsdb.Options{
				RetentionDuration:      int64(*retention),
				WALSegmentSize:         wal.DefaultSegmentSize,
				MinBlockDuration:       tsdb.DefaultBlockDuration,
				MaxBlockDuration:       tsdb.DefaultBlockDuration,
				NoLockfile:             true,
				AllowOverlappingBlocks: false,
				WALCompression:         true,
				StripeSize:             tsdb.DefaultStripeSize,
			},
		)
		if err != nil {
			return err
		}
		return runWeb(mux, logger, db, reloadCh)
	}
}

func runWeb(mux *http.ServeMux, logger log.Logger, db *tsdb.DB, reloadCh chan struct{}) error {
	ui := pprofui.New(log.With(logger, "component", "pprofui"), db)

	router := httprouter.New()
	router.RedirectTrailingSlash = false

	router.GET("/pprof/*remainder", ui.PprofView)
	router.GET("/download/*remainder", ui.PprofDownload)

	api := api.New(log.With(logger, "component", "pprofui"), db, reloadCh)

	router.GET("/-/reload", api.Reload)
	router.GET("/api/v1/query_range", api.QueryRange)

	router.NotFound = http.FileServer(web.Assets)

	mux.Handle("/", router)

	return nil
}
