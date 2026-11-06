//! Chain-agnostic test package: every module reads only the Clock (directly or through
//! other modules), so it runs on the dummy-blockchain devnet and every output is a pure
//! function of the block number. The graph spans four stages, each with several mappers
//! and stores (none on the last one), plus a block index, so that pruned snapshots and
//! deleted outputs at any level are exercised by the stages above.
//!
//!   stage 1: index_parity, map_a, store_count, store_a_max
//!   stage 2: store_sum, store_a_last, map_b
//!   stage 3: store_c, map_c (filtered by index_parity)
//!   stage 4: map_out

use substreams::errors::Error;
use substreams::pb::sf::substreams::index::v1::Keys;
use substreams::pb::substreams::Clock;
use substreams::store::{
    DeltaInt64, Deltas, StoreAdd, StoreAddInt64, StoreGet, StoreGetInt64, StoreMax,
    StoreMaxInt64, StoreNew, StoreSet, StoreSetInt64,
};

fn out(number: u64, id: String) -> Clock {
    Clock { id, number, timestamp: None }
}

#[substreams::handlers::map]
fn index_parity(clock: Clock) -> Result<Keys, Error> {
    let key = if clock.number % 2 == 0 { "even" } else { "odd" };
    Ok(Keys { keys: vec![key.to_string()] })
}

#[substreams::handlers::map]
fn map_a(clock: Clock) -> Result<Clock, Error> {
    Ok(out(clock.number, format!("a:{}", clock.number * 3)))
}

#[substreams::handlers::store]
fn store_count(clock: Clock, s: StoreAddInt64) {
    s.add(clock.number, "count", 1);
    s.add(clock.number, format!("bucket:{}", clock.number / 100), 1);
}

#[substreams::handlers::store]
fn store_a_max(a: Clock, s: StoreMaxInt64) {
    s.max(a.number, "max", (a.number as i64 * 7) % 1013);
}

#[substreams::handlers::store]
fn store_sum(clock: Clock, count: StoreGetInt64, s: StoreAddInt64) {
    let c = count.get_last("count").unwrap_or(0);
    s.add(clock.number, "sum", c + clock.number as i64);
}

#[substreams::handlers::store]
fn store_a_last(a: Clock, amax: StoreGetInt64, s: StoreSetInt64) {
    let m = amax.get_last("max").unwrap_or(0);
    s.set(a.number, "last", &(m + a.number as i64));
}

#[substreams::handlers::map]
fn map_b(clock: Clock, count: StoreGetInt64, amax: StoreGetInt64) -> Result<Clock, Error> {
    let c = count.get_last("count").unwrap_or(0);
    let m = amax.get_last("max").unwrap_or(0);
    Ok(out(clock.number, format!("b:count={} max={}", c, m)))
}

#[substreams::handlers::store]
fn store_c(b: Clock, sum: StoreGetInt64, s: StoreAddInt64) {
    let v = sum.get_last("sum").unwrap_or(0) % 7;
    s.add(b.number, "c", v);
    s.add(b.number, format!("c:{}", b.number / 1000), v);
}

#[substreams::handlers::map]
fn map_c(clock: Clock, alast: StoreGetInt64, sum: StoreGetInt64) -> Result<Clock, Error> {
    let l = alast.get_last("last").unwrap_or(0);
    let s = sum.get_last("sum").unwrap_or(0);
    Ok(out(clock.number, format!("c:last={} sum={}", l, s)))
}

#[substreams::handlers::map]
fn map_out(
    clock: Clock,
    c: StoreGetInt64,
    amax: StoreGetInt64,
    mc: Clock,
    mb: Clock,
    count: StoreGetInt64,
    cdeltas: Deltas<DeltaInt64>,
) -> Result<Clock, Error> {
    let bucket = count
        .get_last(format!("bucket:{}", clock.number / 100))
        .unwrap_or(0);
    Ok(out(
        clock.number,
        format!(
            "c={} c1k={} max={} count={} bucket={} b=[{}] c=[{}] deltas={}",
            c.get_last("c").unwrap_or(0),
            c.get_last(format!("c:{}", clock.number / 1000)).unwrap_or(0),
            amax.get_last("max").unwrap_or(0),
            count.get_last("count").unwrap_or(0),
            bucket,
            mb.id,
            mc.id,
            cdeltas.deltas.len(),
        ),
    ))
}
