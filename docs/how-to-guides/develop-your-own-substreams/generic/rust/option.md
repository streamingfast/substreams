# The Option<T> struct

## The Problem

Consider that you want to implement a function that returns a username, given the corresponding user identifier. The signature of the function could be as follows:

```rust
fn get_username_by_id(id: u32) -> String {
    // function body
}
```

In a success case, you pass the user identifier as a parameter and the function returns the corresponding username. However, **what happens if the function does not have a username for the given identifier?** A possible solution is to return an empty string:
- If the function **is able to retrieve the data**, then the string returned is the username.
- If the function **is NOT able to retrieve the data**, then the string returned is the empty string (`''`).

Although this is a valid approach, it creates hidden logic that is not visible unless you deep dive into the function code.

## The Solution

Rust provides a better way of dealing with these situations by using the `Option<T>` enum. This enum has two possible values: `Some(T)` (used when the returned value is present) and `None` (used when the returned value is not present). Therefore, the previous function can be refactored to:

```rust
fn get_username_by_id(id: u32) -> Option<String> {
    // function body
}
```

Now, the function works as follows:
- If the function **is able to retrieve the data**, then a `Some` value containing the string is returned.
- If the function **is NOT able to retrieve the data**, then a `None` value is returned.

Let's complete the body of the function:

```rust
fn get_username_by_id(id: u32) -> Option<String> { // 1.
    match(id) {
        1 => Some(String::from("Susan")), // 2.
        2 => Some(String::from("John")), // 3.
        _ => None // 4.
    }
}
```
1. Given a user identifier, return the corresponding username if it exists.
2. If `id == 1`, then a `Some` struct containing the string is returned.
3. If `id == 2`, then a `Some` struct containing the string is returned.
4. If `id` does not match with any of the provided identifiers, then a `None` struct is returned.

## Using Options

The `Option<T>` struct contains two helper methods to check if the returned type is `Some` or `None`: the `.is_some()` and `.is_none()` methods. Let's see how to use these methods:

```rust
fn get_username_by_id(id: u32) -> Option<String> {
    match(id) {
        1 => Some(String::from("Susan")),
        2 => Some(String::from("John")),
        _ => None
    }
}

fn main() {
    let user1 = get_username_by_id(1); // 1.
    let user10 = get_username_by_id(10); // 2.

    if (user1.is_some()) { // 3.
        println!("User with id = 1 holds username {}", user1.unwrap())
    }

    if (user10.is_none()) { // 4.
        println!("User with id = 10 does not exist")
    }
}
```
1. Get the user with `id == 1`.
1. Get the user with `id == 10`.
3. If the function returned a name for `id == 1`, then `user1.is_some()` returns `true`.
4. If the function did NOT return a name for `id == 10`, then `user1.is_none()` returns `true`.

You can also use [pattern matching](https://doc.rust-lang.org/book/ch18-03-pattern-syntax.html) instead of the helper methods:

```rust
fn get_username_by_id(id: u32) -> Option<String> {
    match(id) {
        1 => Some(String::from("Susan")),
        2 => Some(String::from("John")),
        _ => None
    }
}

fn main() {
    let user1 = get_username_by_id(1);
    let user10 = get_username_by_id(10);
    
    match (&user1) {
        Some(name) => println!("User with id = 1 holds username {}", &user1.unwrap()),
        None => println!("No user with id = 1 found")
    }
    
    match (&user10) {
        Some(name) => println!("User with id = 10 holds username {}", &user10.unwrap()),
        None => println!("No user with id = 10 found")
    }
}
```
