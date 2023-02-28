package event

import (
	"errors"
	"fmt"

	"github.com/blocklords/gosds/app/remote"
	"github.com/blocklords/gosds/app/remote/message"
)

// Sends the command to the remote SDS Spaghetti to filter the logs
// block_from parameter is either block_number or block_timestamp
// depending on the blockchain
func RemoteLogFilter(socket *remote.Socket, block_from uint64, addresses []string) ([]*Log, error) {
	// Send hello.
	request := message.Request{
		Command: "log-filter",
		Parameters: map[string]interface{}{
			"block_from": block_from,
			"addresses":  addresses,
		},
	}

	params, err := socket.RequestRemoteService(&request)
	if err != nil {
		return nil, fmt.Errorf("socket.RequestRemoteService: %w", err)
	}

	raw_logs, ok := params.ToMap()["logs"].([]interface{})
	if !ok {
		return nil, errors.New("no logs parameter")
	}
	logs, err := NewLogs(raw_logs)
	if err != nil {
		return nil, errors.New("failed to parse log when filtering it")
	}

	return logs, nil
}