package api

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// baseRoute a base dcos-diagnostics endpoint location.
const baseRoute string = "/system/health/v1"

type routeHandler struct {
	url                 string
	handler             func(http.ResponseWriter, *http.Request)
	headers             []header
	methods             []string
	gzip, canFlushCache bool
}

type header struct {
	name  string
	value string
}

func headerMiddleware(next http.Handler, headers []header) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("Connection", "close")
		setJSONContentType := true
		for _, header := range headers {
			if header.name == "Content-type" {
				setJSONContentType = false
			}
			w.Header().Add(header.name, header.value)
		}
		if setJSONContentType {
			w.Header().Add("Content-type", "application/json")
		}
		next.ServeHTTP(w, r)
	})
}

func noCacheMiddleware(next http.Handler, dt *Dt) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cache := r.URL.Query()["cache"]; len(cache) == 0 {
			if t := dt.MR.GetLastUpdatedTime(); t != "" {
				w.Header().Set("Last-Modified-3DT", t)
			}
			next.ServeHTTP(w, r)
			return
		}

		if !dt.Cfg.FlagPull {
			e := "dcos-diagnostics was not started with -pull flag"
			logrus.Error(e)
			http.Error(w, e, http.StatusServiceUnavailable)
			return
		}

		dt.RunPullerChan <- true
		select {
		case <-dt.RunPullerDoneChan:
			logrus.Debug("Fresh data updated")

		case <-time.After(time.Minute):
			panic("Error getting fresh health report")
		}

		if t := dt.MR.GetLastUpdatedTime(); t != "" {
			w.Header().Set("Last-Modified-3DT", t)
		}
		next.ServeHTTP(w, r)
	})
}

func getRoutes(dt *Dt) []routeHandler {
	h := handler{
		cfg:                dt.Cfg,
		tools:              dt.DtDCOSTools,
		job:                dt.DtDiagnosticsJob,
		systemdUnits:       dt.SystemdUnits,
		monitoringResponse: dt.MR,
	}
	routes := []routeHandler{
		{
			// /system/health/v1
			url:     baseRoute,
			handler: h.unitsHealthStatus,
		},
		{
			// /system/health/v1/report
			url:           fmt.Sprintf("%s/report", baseRoute),
			handler:       h.reportHandler,
			canFlushCache: true,
		},
		{
			// /system/health/v1/report/download
			url:     fmt.Sprintf("%s/report/download", baseRoute),
			handler: h.reportHandler,
			headers: []header{
				{
					name:  "Content-disposition",
					value: "attachment; filename=health-report.json",
				},
			},
			canFlushCache: true,
		},
		{
			// /system/health/v1/units
			url:           fmt.Sprintf("%s/units", baseRoute),
			handler:       h.getAllUnitsHandler,
			canFlushCache: true,
		},
		{
			// /system/health/v1/units/<unitid>
			url:           fmt.Sprintf("%s/units/{unitid}", baseRoute),
			handler:       h.getUnitByIDHandler,
			canFlushCache: true,
		},
		{
			// /system/health/v1/units/<unitid>/nodes
			url:           fmt.Sprintf("%s/units/{unitid}/nodes", baseRoute),
			handler:       h.getNodesByUnitIDHandler,
			canFlushCache: true,
		},
		{
			// /system/health/v1/units/<unitid>/nodes/<nodeid>
			url:           fmt.Sprintf("%s/units/{unitid}/nodes/{nodeid}", baseRoute),
			handler:       h.getNodeByUnitIDNodeIDHandler,
			canFlushCache: true,
		},
		{
			// /system/health/v1/nodes
			url:           fmt.Sprintf("%s/nodes", baseRoute),
			handler:       h.getNodesHandler,
			canFlushCache: true,
		},
		{
			// /system/health/v1/nodes/<nodeid>
			url:           fmt.Sprintf("%s/nodes/{nodeid}", baseRoute),
			handler:       h.getNodeByIDHandler,
			canFlushCache: true,
		},
		{
			// /system/health/v1/nodes/<nodeid>/units
			url:           fmt.Sprintf("%s/nodes/{nodeid}/units", baseRoute),
			handler:       h.getNodeUnitsByNodeIDHandler,
			canFlushCache: true,
		},
		{
			// /system/health/v1/nodes/<nodeid>/units/<unitid>
			url:           fmt.Sprintf("%s/nodes/{nodeid}/units/{unitid}", baseRoute),
			handler:       h.getNodeUnitByNodeIDUnitIDHandler,
			canFlushCache: true,
		},

		// diagnostics routes
		{
			// /system/health/v1/logs
			url:     baseRoute + "/logs",
			handler: h.logsListHandler,
		},
		{
			// /system/health/v1/logs/<unitid/<hours>
			url:     baseRoute + "/logs/{provider}/{entity}",
			handler: h.getUnitLogHandler,
			headers: []header{
				{
					name:  "Content-type",
					value: "text/html",
				},
			},
			gzip: true,
		},
		{
			// /system/health/v1/report/diagnostics
			url:     baseRoute + "/report/diagnostics/create",
			handler: h.createBundleHandler,
			methods: []string{"POST"},
		},
		{
			url:     baseRoute + "/report/diagnostics/cancel",
			handler: h.cancelBundleReportHandler,
			methods: []string{"POST"},
		},
		{
			url:     baseRoute + "/report/diagnostics/status",
			handler: h.diagnosticsJobStatusHandler,
		},
		{
			url:     baseRoute + "/report/diagnostics/status/all",
			handler: h.diagnosticsJobStatusAllHandler,
		},
		{
			// /system/health/v1/report/diagnostics/list
			url:     baseRoute + "/report/diagnostics/list",
			handler: h.listAvailableLocalBundlesFilesHandler,
		},
		{
			// /system/health/v1/report/diagnostics/list/all
			url:     baseRoute + "/report/diagnostics/list/all",
			handler: h.listAvailableGLobalBundlesFilesHandler,
		},
		{
			// /system/health/v1/report/diagnostics/serve/<file>
			url:     baseRoute + "/report/diagnostics/serve/{file}",
			handler: h.downloadBundleHandler,
			headers: []header{
				{
					name:  "Content-type",
					value: "application/octet-stream",
				},
			},
		},
		{
			// /system/health/v1/report/diagnostics/delete/<file>
			url:     baseRoute + "/report/diagnostics/delete/{file}",
			handler: h.deleteBundleHandler,
			methods: []string{"POST"},
		},
		// self test route
		{
			url:     baseRoute + "/selftest/info",
			handler: h.selfTestHandler,
		},
	}

	if dt.Cfg.FlagDebug {
		logrus.Debug("Enabling pprof endpoints.")
		routes = append(routes, []routeHandler{
			{
				url:     baseRoute + "/debug/pprof/",
				handler: pprof.Index,
				gzip:    true,
				headers: []header{
					{
						name:  "Content-type",
						value: "text/html",
					},
				},
			},
			{
				url:     baseRoute + "/debug/pprof/cmdline",
				handler: pprof.Cmdline,
				gzip:    true,
				headers: []header{
					{
						name:  "Content-type",
						value: "text/html",
					},
				},
			},
			{
				url:     baseRoute + "/debug/pprof/profile",
				handler: pprof.Profile,
				gzip:    true,
				headers: []header{
					{
						name:  "Content-type",
						value: "text/html",
					},
				},
			},
			{
				url:     baseRoute + "/debug/pprof/symbol",
				handler: pprof.Symbol,
				gzip:    true,
				headers: []header{
					{
						name:  "Content-type",
						value: "text/html",
					},
				},
			},
			{
				url:     baseRoute + "/debug/pprof/trace",
				handler: pprof.Trace,
				gzip:    true,
				headers: []header{
					{
						name:  "Content-type",
						value: "text/html",
					},
				},
			},
			{
				url: baseRoute + "/debug/pprof/{profile}",
				handler: func(w http.ResponseWriter, req *http.Request) {
					profile := mux.Vars(req)["profile"]
					pprof.Handler(profile).ServeHTTP(w, req)
				},
				gzip: true,
				headers: []header{
					{
						name:  "Content-type",
						value: "text/html",
					},
				},
			},
		}...)
	}

	return routes
}

func wrapHandler(handler http.Handler, route routeHandler, dt *Dt) http.Handler {
	h := headerMiddleware(handler, route.headers)
	if route.gzip {
		h = handlers.CompressHandler(h)
	}
	if route.canFlushCache {
		h = noCacheMiddleware(h, dt)
	}

	return handlers.LoggingHandler(logrus.StandardLogger().Out, h)
}

func loadRoutes(router *mux.Router, dt *Dt) *mux.Router {
	for _, route := range getRoutes(dt) {
		if len(route.methods) == 0 {
			route.methods = []string{"GET"}
		}
		handler := http.HandlerFunc(route.handler)
		router.Handle(route.url, wrapHandler(handler, route, dt)).Methods(route.methods...)
	}
	return router
}

// NewRouter returns a new *mux.Router with loaded routes.
func NewRouter(dt *Dt) *mux.Router {
	router := mux.NewRouter().StrictSlash(true)
	return loadRoutes(router, dt)
}
