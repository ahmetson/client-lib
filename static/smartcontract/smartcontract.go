package smartcontract

import (
	"github.com/blocklords/sds/common/blockchain"
	"github.com/blocklords/sds/common/smartcontract_key"
)

type Smartcontract struct {
	Key            smartcontract_key.Key     `json:"key"`
	AbiId          string                    `json:"abi_id"`
	TransactionKey blockchain.TransactionKey `json:"transaction_key"`
	Block          blockchain.Block          `json:"block"`
	Deployer       string                    `json:"deployer"`
}
