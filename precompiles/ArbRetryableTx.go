//
// Copyright 2021, Offchain Labs, Inc. All rights reserved.
//

package precompiles

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/params"
	"github.com/offchainlabs/arbstate/arbos/retryables"
	"github.com/offchainlabs/arbstate/util"
)

type ArbRetryableTx struct {
	Address                 addr
	TicketCreated           func(mech, [32]byte)
	LifetimeExtended        func(mech, [32]byte, huge)
	RedeemScheduled         func(mech, [32]byte, [32]byte, uint64, uint64, addr)
	Redeemed                func(mech, [32]byte)
	Canceled                func(mech, [32]byte)
	TicketCreatedGasCost    func([32]byte) uint64
	LifetimeExtendedGasCost func([32]byte, huge) uint64
	RedeemScheduledGasCost  func([32]byte, [32]byte, uint64, uint64, addr) uint64
	RedeemedGasCost         func([32]byte) uint64
	CanceledGasCost         func([32]byte) uint64
}

var NotFoundError = errors.New("ticketId not found")

func (con ArbRetryableTx) Cancel(c ctx, evm mech, ticketId [32]byte) error {
	retryableState := c.state.RetryableState()
	retryable, err := retryableState.OpenRetryable(ticketId, evm.Context.Time.Uint64())
	if err != nil {
		return err
	}
	if retryable == nil {
		return NotFoundError
	}
	beneficiary, err := retryable.Beneficiary()
	if err != nil {
		return err
	}
	if c.caller != beneficiary {
		return errors.New("only the beneficiary may cancel a retryable")
	}

	// no refunds are given for deleting retryables because they use rented space
	_, err = retryableState.DeleteRetryable(ticketId)
	con.Canceled(evm, ticketId)
	return err
}

func (con ArbRetryableTx) GetBeneficiary(c ctx, evm mech, ticketId [32]byte) (addr, error) {
	if err := c.burn(2 * params.SloadGas); err != nil {
		return addr{}, err
	}
	retryableState := c.state.RetryableState()
	retryable, err := retryableState.OpenRetryable(ticketId, evm.Context.Time.Uint64())
	if err != nil {
		return addr{}, err
	}
	if retryable == nil {
		return addr{}, NotFoundError
	}
	return retryable.Beneficiary()
}

func (con ArbRetryableTx) GetLifetime(c ctx, evm mech) (huge, error) {
	// there's no need to burn gas for something this cheap
	return big.NewInt(retryables.RetryableLifetimeSeconds), nil
}

func (con ArbRetryableTx) GetTimeout(c ctx, evm mech, ticketId [32]byte) (huge, error) {
	retryableState := c.state.RetryableState()
	retryable, err := retryableState.OpenRetryable(ticketId, evm.Context.Time.Uint64())
	if err != nil {
		return nil, err
	}
	if retryable == nil {
		return nil, NotFoundError
	}
	timeout, err := retryable.Timeout()
	if err != nil {
		return nil, err
	}
	return big.NewInt(int64(timeout)), nil
}

func (con ArbRetryableTx) Keepalive(c ctx, evm mech, ticketId [32]byte) (huge, error) {

	// charge for the check & event
	eventCost := con.LifetimeExtendedGasCost(ticketId, big.NewInt(0))
	if err := c.burn(6*params.SloadGas + 2*params.SstoreSetGas + eventCost); err != nil {
		return big.NewInt(0), err
	}

	// charge for the expiry update
	retryableState := c.state.RetryableState()
	nbytes, err := retryableState.RetryableSizeBytes(ticketId, evm.Context.Time.Uint64())
	if err != nil {
		return nil, err
	}
	if nbytes == 0 {
		return nil, NotFoundError
	}
	updateCost := util.WordsForBytes(nbytes) * params.SstoreSetGas / 100
	if err := c.burn(updateCost); err != nil {
		return big.NewInt(0), err
	}

	currentTime := evm.Context.Time.Uint64()
	window := currentTime + retryables.RetryableLifetimeSeconds
	err = retryableState.Keepalive(ticketId, currentTime, window, retryables.RetryableLifetimeSeconds)
	if err != nil {
		return big.NewInt(0), err
	}

	retryable, err := retryableState.OpenRetryable(ticketId, currentTime)
	if err != nil {
		return nil, err
	}
	newTimeout, err := retryable.Timeout()
	if err != nil {
		return nil, err
	}
	con.LifetimeExtended(evm, ticketId, big.NewInt(int64(newTimeout)))
	return big.NewInt(int64(newTimeout)), nil
}

func (con ArbRetryableTx) Redeem(c ctx, evm mech, ticketId [32]byte) ([32]byte, error) {

	eventCost := con.RedeemScheduledGasCost(ticketId, ticketId, 0, 0, c.caller)
	if err := c.burn(5*params.SloadGas + params.SstoreSetGas + eventCost); err != nil {
		return hash{}, err
	}

	retryableState := c.state.RetryableState()
	byteCount, err := retryableState.RetryableSizeBytes(ticketId, evm.Context.Time.Uint64())
	if err != nil {
		return hash{}, err
	}
	writeBytes := util.WordsForBytes(byteCount)
	if err := c.burn(params.SloadGas * writeBytes); err != nil {
		return hash{}, err
	}

	retryable, err := retryableState.OpenRetryable(ticketId, evm.Context.Time.Uint64())
	if err != nil {
		return hash{}, err
	}
	if retryable == nil {
		return hash{}, NotFoundError
	}
	sequenceNum, err := retryable.IncrementNumTries()
	if err != nil {
		return hash{}, err
	}
	redeemTxId := retryables.TxIdForRedeemAttempt(ticketId, sequenceNum)
	con.RedeemScheduled(evm, ticketId, redeemTxId, sequenceNum, c.gasLeft, c.caller)

	// now donate all of the remaining gas to the retry
	// to do this, we burn the gas here, but add it back into the gas pool just before the retry runs
	// the gas payer for this transaction will get a credit for the wei they paid for this gas, when the retry occurs
	if err := c.burn(c.gasLeft); err != nil {
		return hash{}, err
	}

	return redeemTxId, nil
}
