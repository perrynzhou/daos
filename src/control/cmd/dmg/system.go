//
// (C) Copyright 2019-2020 Intel Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// GOVERNMENT LICENSE RIGHTS-OPEN SOURCE SOFTWARE
// The Government's rights to use, modify, reproduce, release, perform, display,
// or disclose this software are subject to the terms of the Apache License as
// provided in Contract No. 8F-30005.
// Any reproduction of computer software, computer software documentation, or
// portions thereof marked with this legend must also reproduce the markings.
//

package main

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/daos-stack/daos/src/control/client"
	"github.com/daos-stack/daos/src/control/lib/hostlist"
	"github.com/daos-stack/daos/src/control/lib/txtfmt"
)

// SystemCmd is the struct representing the top-level system subcommand.
type SystemCmd struct {
	LeaderQuery leaderQueryCmd     `command:"leader-query" alias:"l" description:"Query for current Management Service leader"`
	Query       systemQueryCmd     `command:"query" alias:"q" description:"Query DAOS System Membership"`
	Stop        systemStopCmd      `command:"stop" alias:"s" description:"Perform controlled shutdown of DAOS system"`
	Start       systemStartCmd     `command:"start" alias:"r" description:"Perform start of stopped DAOS system"`
	ListPools   systemListPoolsCmd `command:"list-pools" alias:"p" description:"List all pools in the DAOS system"`
}

type leaderQueryCmd struct {
	logCmd
	cfgCmd
	connectedCmd
}

func (cmd *leaderQueryCmd) Execute(_ []string) error {
	resp, err := cmd.conns.LeaderQuery(client.LeaderQueryReq{
		System: cmd.config.SystemName,
	})
	if err != nil {
		return errors.Wrap(err, "leader query failed")
	}

	cmd.log.Infof("Current Leader: %s\n   Replica Set: %s\n", resp.Leader,
		strings.Join(resp.Replicas, ", "))
	return nil
}

// systemQueryCmd is the struct representing the command to list
// system member details.
type systemQueryCmd struct {
	logCmd
	connectedCmd
}

// Execute is run when systemQueryCmd activates
func (cmd *systemQueryCmd) Execute(args []string) error {
	members, err := cmd.conns.SystemQuery()
	if err != nil {
		return errors.Wrap(err, "System-Query command failed")
	}

	cmd.log.Debug("System-Query command succeeded\n")
	if len(members) == 0 {
		cmd.log.Info("No members in system\n")
		return nil
	}

	rankTitle := "Rank"
	uuidTitle := "UUID"
	addrTitle := "Control Address"
	stateTitle := "State"

	formatter := txtfmt.NewTableFormatter(rankTitle, uuidTitle, addrTitle, stateTitle)
	var table []txtfmt.TableRow

	for _, m := range members {
		row := txtfmt.TableRow{rankTitle: fmt.Sprintf("%d", m.Rank)}
		row[uuidTitle] = m.UUID
		row[addrTitle] = m.Addr.String()
		row[stateTitle] = m.State().String()

		table = append(table, row)
	}

	cmd.log.Info(formatter.Format(table))

	return nil
}

// systemStopCmd is the struct representing the command to shutdown DAOS system.
type systemStopCmd struct {
	logCmd
	connectedCmd
	Prep bool `long:"prep" description:"Perform prep phase of controlled shutdown."`
	Kill bool `long:"kill" description:"Perform kill phase of controlled shutdown."`
}

// Execute is run when systemStopCmd activates
func (cmd *systemStopCmd) Execute(args []string) error {
	if !cmd.Prep && !cmd.Kill {
		cmd.Prep = true
		cmd.Kill = true
	}

	req := client.SystemStopReq{Prep: cmd.Prep, Kill: cmd.Kill}
	results, err := cmd.conns.SystemStop(req)
	if err != nil {
		return errors.Wrap(err, "System-Stop command failed")
	}

	if len(results) == 0 {
		cmd.log.Debug("System-Stop no member results returned\n")
		return nil
	}
	cmd.log.Debug("System-Stop command succeeded\n")

	groups := make(hostlist.HostGroups)

	for _, r := range results {
		msg := "OK"
		if r.Err != nil {
			msg = r.Err.Error()
		}
		resStr := fmt.Sprintf("%s%s%s", r.Action, rowFieldSep, msg)
		if err = groups.AddHost(resStr, fmt.Sprintf("rank%d", r.Rank)); err != nil {
			return errors.Wrap(err, "adding rank result to group")
		}
	}

	out, err := tabulateHostGroups(groups, "Ranks", "Operation", "Result")
	if err != nil {
		return errors.Wrap(err, "printing result table")
	}

	cmd.log.Info(out)

	return nil
}

// systemStartCmd is the struct representing the command to start system.
type systemStartCmd struct {
	logCmd
	connectedCmd
}

// Execute is run when systemStartCmd activates
func (cmd *systemStartCmd) Execute(args []string) error {
	msg := "SUCCEEDED: "

	err := cmd.conns.SystemStart()
	if err != nil {
		msg = errors.WithMessagef(err, "FAILED").Error()
	}

	cmd.log.Infof("System-start command %s\n", msg)

	return nil
}

// Execute is run when systemListPoolsCmd activates
// systemListPoolsCmd represents the command to fetch a list of all DAOS pools in the system.
type systemListPoolsCmd struct {
	logCmd
	connectedCmd
	cfgCmd
}

func formatPoolSvcReps(svcReps []uint32) string {
	var b strings.Builder
	for i, rep := range svcReps {
		if i != 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%d", rep)
	}

	return b.String()
}

// Execute is run when systemListPoolsCmd activates
func (cmd *systemListPoolsCmd) Execute(args []string) error {
	if cmd.config == nil {
		return errors.New("No configuration loaded")
	}
	req := client.ListPoolsReq{SysName: cmd.config.SystemName}
	resp, err := cmd.conns.ListPools(req)
	if err != nil {
		return errors.Wrap(err, "List-Pools command failed")
	}

	cmd.log.Debug("List-Pools command succeeded\n")
	if len(resp.Pools) == 0 {
		cmd.log.Info("No pools in system\n")
		return nil
	}

	uuidTitle := "Pool UUID"
	svcRepTitle := "Svc Replicas"

	formatter := txtfmt.NewTableFormatter(uuidTitle, svcRepTitle)
	var table []txtfmt.TableRow

	for _, pool := range resp.Pools {
		row := txtfmt.TableRow{uuidTitle: pool.UUID}

		if len(pool.SvcReplicas) != 0 {
			row[svcRepTitle] = formatPoolSvcReps(pool.SvcReplicas)
		}

		table = append(table, row)
	}

	cmd.log.Info(formatter.Format(table))
	return nil
}
