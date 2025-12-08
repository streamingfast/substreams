# The Result<T, E> struct

In Rust, the `Result<T, E>` struct is used to abstract both a successful response (if it exists) and an error (if it occurs). Let's better understand through an example.

## Basic Usage

Consider that you have a function `divide(num1, num2)`, which executes the division between two numbers. As you already know, dividing by 0 is undefined, and generates an error in Rust. You can use `Result` to return a controlled error.

```rust
fn divide(num1: u32, num2: u32) -> Result<u32, String> {
    if num2 == 0 {
        return Err(String::from("You can't divide by 0"));
    }

    return Ok(num1 / num2);
}

fn main() {
    let result = divide(6, 0);
    if result.is_ok() {
        println!("This is the happy path: {}", result.unwrap())
    } else {
        println!("This is the error: {}", result.err().unwrap())
    }
}
```

Let's inspect the `divide` function:

```rust
fn divide(num1: u32, num2: u32) -> Result<u32, String> { // 1.
    if num2 == 0 {
        return Err(String::from("You can't divide by 0")); // 2.
    }

    return Ok(num1 / num2); // 3.
}
```

1. Declaration of the function. Two unsigned numbers of 32-bit length are passed as parameters.
   The return type is `Result<u32, String>`: the first type (`u32`) is for the successful response, and the second type (`String`) is for the error response.
2. If dividing by 0, you return an error String.
3. If not, you return the result of the division (`u32`).

The `Result<T, E>` is really an enum that can take two values: `Ok(T)` (success) and `Err(E)` (error).

In the previous code, when you return `Err(String)`, the success part is automatically empty. At the same time, when you return `Ok(u32)`, the error part is empty.

Now, let's see how you can interact with this result.

```rust
fn main() {
    let result = divide(6, 0); // 1.
    if result.is_ok() { // 2.
        println!("This is the happy path: {}", result.unwrap())
    } else { // 3.
        println!("This is the error: {}", result.err().unwrap())
    }
}
```

1. You invoke the function and store the `Result<T,E>` enum in a variable.
2. If the result _is ok_ (i.e. the happy path has been returned), you can take its value by using the `result.unwrap()` method.
3. If the error has been returned, you can return the error string by using the `result.err().unwrap()` method.

The output of the program for `divide(6,2)` (happy path) is:

```bash
This is the happy path: 3
```

The output of the program for `divide(6,0)` (error) is:

```bash
This is the error: You can't divide by 0
```

## The Shortcut

Checking with an `if` condition whether the result contains an error is a valid approach. However, Rust includes a shortcut to improve this.

In the previous example, consider that you want to invoke the `divide` function from another function that performs other computations.

```rust
fn divide(num1: u32, num2: u32) -> Result<u32, String> {
    if num2 == 0 {
        return Err(String::from("You can't divide by 0"));
    }

    return Ok(num1 / num2);
}

fn computations() -> Result<u32, String> {
    let result = divide(6, 0); // Performing the division

    if result.is_err() { // If the division returns an error, then you return an error.
        return Err(result.err().unwrap());
    }

    let division_result = result.unwrap();
    return Ok(division_result + 5);
}

fn main() {
    let result = computations();
    if result.is_ok() {
        println!("This is the happy path: {}", result.unwrap())
    } else {
        println!("This is the error: {}", result.err().unwrap())
    }
}
```

Now, the Rust program adds `5` to the result of the division, checking that the division is correct first.
Although this approach is correct, Rust provides a `?` symbol that simplifies the logic:

```rust
fn divide(num1: u32, num2: u32) -> Result<u32, String> {
    if num2 == 0 {
        return Err(String::from("You can't divide by 0"));
    }

    return Ok(num1 / num2);
}

fn computations() -> Result<u32, String> {
    let division_result = divide(6, 0)?;

    return Ok(division_result + 5);
}

fn main() {
    let result = computations();
    if result.is_ok() {
        println!("This is the happy path: {}", result.unwrap())
    } else {
        println!("This is the error: {}", result.err().unwrap())
    }
}
```

The `?` symbol after a `Result` enum does two things:

1. If successful, it unwraps the result (in this case, a `u32` number), and stores it in a variable
   `let division_result = divide(6, 0)?;`
2. If an error occurs, it returns the error directly. In this example, the error type of the `divide` and the `computations` function is the same (a `String`).

## In Substreams

The `Result` enum is used in Substreams to return the data (or the errors) of a module. For example, if you take the `map_filter_transactions` module from the [Ethereum Explorer tutorial](/tutorials/ethereum/exploring-ethereum/map_filter_transactions_module):

```rust
[...]

#[substreams::handlers::map]
fn map_filter_transactions(params: String, blk: Block) -> Result<Transactions, Vec<substreams::errors::Error>> {
    let filters = parse_filters_from_params(params)?;

    let transactions: Vec<Transaction> = blk
        .transactions()
        .filter(|trans| apply_filter(&trans, &filters))
        .map(|trans| Transaction {
            from: Hex::encode(&trans.from),
            to: Hex::encode(&trans.to),
            hash: Hex::encode(&trans.hash),
        })
        .collect();

    Ok(Transactions { transactions })
}

[...]
```

This module returns a `Result<Transactions, Vec<substreams::errors::Error>> ` enum. If successful, it returns the transactions filtered; in the case of an error, it returns the `substreams::errors::Error` error, which is a Substreams wrapper for a generic [anyhow Rust error](https://docs.rs/anyhow/latest/anyhow/).
