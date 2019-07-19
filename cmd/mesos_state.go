package cmd

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
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
			return fmt.Errorf("could not GET %s: %s", stateURL, err)
		}

		raw, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("could not read response: %s", err)
		}

		fmt.Print(string(raw))
		return nil
	},
}
