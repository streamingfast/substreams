// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract Counter {
    uint256 private count;
    address public owner;

    event Incremented(uint256 newCount, address caller);
    event Decremented(uint256 newCount, address caller);

    constructor(uint256 initialCount) {
        count = initialCount;
        owner = msg.sender;
    }

    function increment() public {
        count += 1;
        emit Incremented(count, msg.sender);
    }

    function decrement() public {
        require(count > 0, "Counter: cannot decrement below zero");
        count -= 1;
        emit Decremented(count, msg.sender);
    }

    function getCount() public view returns (uint256) {
        return count;
    }
}
