//
// Copyright 2021, Offchain Labs, Inc. All rights reserved.
//

package precompiles

import (
	"errors"
)

type ArbAggregator struct {
	Address addr
}

func (con ArbAggregator) GetFeeCollector(c ctx, evm mech, aggregator addr) (addr, error) {
	return c.state.L1PricingState().AggregatorFeeCollector(aggregator)
}

func (con ArbAggregator) GetDefaultAggregator(c ctx, evm mech) (addr, error) {
	return c.state.L1PricingState().DefaultAggregator()
}

func (con ArbAggregator) GetPreferredAggregator(c ctx, evm mech, address addr) (addr, bool, error) {
	return c.state.L1PricingState().PreferredAggregator(address)
}

func (con ArbAggregator) GetTxBaseFee(c ctx, evm mech, aggregator addr) (huge, error) {
	return c.state.L1PricingState().FixedChargeForAggregatorL1Gas(aggregator)
}

func (con ArbAggregator) SetFeeCollector(c ctx, evm mech, aggregator addr, newFeeCollector addr) error {
	l1State := c.state.L1PricingState()
	collector, err := l1State.AggregatorFeeCollector(aggregator)
	if err != nil {
		return err
	}
	if (c.caller != aggregator) && (c.caller != collector) {
		// only the aggregator and its current fee collector can change the aggregator's fee collector
		return errors.New("non-authorized c.caller in ArbAggregator.SetFeeCollector")
	}
	return l1State.SetAggregatorFeeCollector(aggregator, newFeeCollector)
}

func (con ArbAggregator) SetDefaultAggregator(c ctx, evm mech, newDefault addr) error {
	return c.state.L1PricingState().SetDefaultAggregator(newDefault)
}

func (con ArbAggregator) SetPreferredAggregator(c ctx, evm mech, prefAgg addr) error {
	return c.state.L1PricingState().SetPreferredAggregator(c.caller, prefAgg)
}

func (con ArbAggregator) SetTxBaseFee(c ctx, evm mech, aggregator addr, feeInL1Gas huge) error {
	return c.state.L1PricingState().SetFixedChargeForAggregatorL1Gas(aggregator, feeInL1Gas)
}
