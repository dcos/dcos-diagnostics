package cmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/dcos/dcos-diagnostics/util"
	"github.com/dcos/dcos-go/dcos"
	"github.com/spf13/cobra"
)

// daemonCmd represents the daemon command
var stateCmd = &cobra.Command{
	Use:   "mesos-state",
	Short: "Query Mesos for its state and print the results to stdout",
	RunE: func(cmd *cobra.Command, args []string) error {
		tr, err := initTransport()
		if err != nil {
			return err
		}

		return getMesosState(tr, os.Stdout)
	},
}

func getMesosState(tr http.RoundTripper, out io.Writer) error {
	defaultStateURL := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(dcos.DNSRecordLeader, strconv.Itoa(dcos.PortMesosMaster)),
		Path:   "/state",
	}

	stateURL, err := util.UseTLSScheme(defaultStateURL.String(), defaultConfig.FlagForceTLS)
	if err != nil {
		return err
	}
	client := util.NewHTTPClient(defaultConfig.GetHTTPTimeout(), tr)

	resp, err := client.Get(stateURL)
	if err != nil {
		return err
	}

	raw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("could not read response: %s", err)
	}

	_, err = out.Write(raw)
	if err != nil {
		return fmt.Errorf("could not wrtie output: %s", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}
