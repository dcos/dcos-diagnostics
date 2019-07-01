package rest

import (
	"context"
	"path/filepath"
)

/*
func TestIfCreateRemoteBundleReturns400WhenEmptyNodeListGiven(t *testing.T) {
	t.Parallel()
	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)

	bh := NewBundleHandler(workdir, nil, nil, time.Millisecond, nil)

	args := createArguments{
		BundleType: bundleTypeRemote,
		Nodes:      []node{},
	}

	body, err := json.Marshal(args)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, bundlesEndpoint+"/bundle-0", bytes.NewReader(body))

	require.NoError(t, err)

	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Create)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.JSONEq(t, `{"code":400,"error":"must include list of nodes to create remote bundle"}`, rr.Body.String())
}

func TestRemoteBundleCreation(t *testing.T) {
	workdir, err := ioutil.TempDir("", "work-dir")
	require.NoError(t, err)
	err = os.RemoveAll(workdir) // just check if dcos-diagnostics will create whole path to workdir
	require.NoError(t, err)

	now, err := time.Parse(time.RFC3339, "2015-08-05T08:40:51.620Z")
	require.NoError(t, err)

	coordinator := mockCoordinator{}

	bh := NewBundleHandler(
		workdir,
		nil,
		coordinator,
		time.Second,
		MockURLBuilder{},
	)
	bh.clock = &MockClock{now: now}

	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Create).Methods(http.MethodPut)
	router.HandleFunc(bundleEndpoint, bh.Get).Methods(http.MethodGet)
	router.HandleFunc(bundleEndpoint, bh.Delete).Methods(http.MethodDelete)
	router.HandleFunc(bundleFileEndpoint, bh.GetFile).Methods(http.MethodGet)

	t.Run("send creation request", func(t *testing.T) {
		// roles will be ignored by the mock url builder
		payload := createArguments{
			BundleType: "REMOTE",
			Nodes: []node{
				node{
					IP:   net.IPv4(192, 0, 2, 1),
					Role: "agent",
				},
				node{
					IP:   net.IPv4(192, 0, 2, 2),
					Role: "master",
				},
				node{
					IP:   net.IPv4(192, 0, 2, 3),
					Role: "agent",
				},
			},
		}

		req, err := http.NewRequest(http.MethodPut, bundlesEndpoint+"/bundle-0", bytes.NewBuffer(jsonMarshal(payload)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, string(jsonMarshal(Bundle{
			ID:      "bundle-0",
			Status:  Started,
			Started: now.Add(time.Hour),
		})), rr.Body.String())

	})

	t.Run("get status", func(t *testing.T) {
		rr := httptest.NewRecorder()

		for { // busy wait for bundle
			time.Sleep(time.Millisecond)
			req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle-0", nil)
			require.NoError(t, err)

			rr = httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if strings.Contains(rr.Body.String(), Done.String()) {
				break
			}
		}

		req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle-0", nil)
		require.NoError(t, err)

		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, string(jsonMarshal(Bundle{
			ID:      "bundle-0",
			Size:    727,
			Status:  Done,
			Started: now.Add(time.Hour),
			Stopped: now.Add(2 * time.Hour),
			Errors:  []string{},
		})), rr.Body.String())
	})

	t.Run("get bundle-0 file and validate it", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle-0/file", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		reader, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(len(rr.Body.Bytes())))
		require.NoError(t, err)

		expectedContents := []string{
			"192.0.2.1/",
			"192.0.2.1/test.txt",
			"192.0.2.2/",
			"192.0.2.2/test.txt",
			"192.0.2.3/",
			"192.0.2.3/test.txt",
		}

		filenames := []string{}
		for _, f := range reader.File {
			filenames = append(filenames, f.Name)
		}
		sort.Strings(filenames)

		assert.Equal(t, expectedContents, filenames)
	})
}
*/
type mockCoordinator struct{}

func (c mockCoordinator) CreateBundle(ctx context.Context, id string, nodes []node) <-chan BundleStatus {
	statuses := make(chan BundleStatus, len(nodes))

	for _, n := range nodes {
		node := n
		statuses <- BundleStatus{
			id:   id,
			node: node,
			done: true,
		}
	}

	return statuses
}

func (c mockCoordinator) CollectBundle(ctx context.Context, id string, numBundles int, statuses <-chan BundleStatus) (string, error) {
	return filepath.Abs(filepath.Join("testdata", "combined.zip"))
}
