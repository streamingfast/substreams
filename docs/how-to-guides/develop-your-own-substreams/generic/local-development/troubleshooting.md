# Local Development Troubleshooting

This guide covers common issues you might encounter when setting up local blockchain environments for Substreams development.

## Docker Compose Issues

### Container Fails to Start

**Problem:** Container exits immediately or fails to start

**Solutions:**
```bash
# Check container logs
docker compose logs <service-name>

# Check all container statuses
docker compose ps

# Restart services
docker compose down
docker compose up -d

# Clean restart (removes volumes)
docker compose down --volumes
docker compose up -d
```

### Port Conflicts

**Problem:** "Port already in use" errors

**Solutions:**
```bash
# Check what's using the ports
lsof -i :8545  # Ethereum RPC
lsof -i :8089  # Firehose
lsof -i :9000  # Substreams
lsof -i :8899  # Solana RPC

# Kill conflicting processes
sudo kill -9 <PID>

# Or change ports in docker-compose.yml
```

### Volume Permission Issues

**Problem:** Permission denied errors in containers

**Solutions:**
```bash
# Reset volume permissions
docker compose down --volumes
sudo chown -R $USER:$USER ./data

# Or recreate volumes
docker volume rm <volume-name>
docker compose up -d
```

### Memory/Resource Issues

**Problem:** Containers running out of memory

**Solutions:**
- Increase Docker Desktop memory allocation (4GB minimum recommended)
- Close other resource-intensive applications
- Check available disk space (20GB minimum recommended)

## RPC Connectivity Problems

### Connection Refused

**Problem:** Cannot connect to localhost:8545 (Ethereum) or localhost:8899 (Solana)

**Solutions:**
1. **Verify container is running and healthy:**
   ```bash
   docker compose ps
   # Look for "healthy" status
   ```

2. **Check port mapping:**
   ```bash
   docker compose logs <service-name>
   # Look for "listening on" messages
   ```

3. **Test basic connectivity:**
   ```bash
   # Ethereum
   curl -X POST http://localhost:8545 \
     -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
   
   # Solana
   curl http://localhost:8899 \
     -X POST -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","id":1,"method":"getHealth"}'
   ```

### Invalid JSON-RPC Response

**Problem:** RPC returns errors or invalid responses

**Solutions:**
- Wait for container to fully initialize (check health status)
- Verify the blockchain node is running with correct parameters
- Check container logs for initialization errors

## Substreams gRPC Issues

### Connection Failed

**Problem:** Cannot connect to Substreams endpoint

**Solutions:**
1. **Verify Substreams service is running:**
   ```bash
   docker compose logs | grep -i substreams
   ```

2. **Test connectivity:**
   ```bash
   substreams run -e localhost:9000 --plaintext common@v0.1.0 -o clock -s -1
   ```

3. **Check firewall settings:**
   - Ensure port 9000 is not blocked
   - Disable VPN if causing connectivity issues

### Authentication Errors

**Problem:** gRPC authentication failures

**Solutions:**
- Always use `--plaintext` flag for local development
- Ensure no API keys are being used for local endpoints
- Verify endpoint URL format (no https:// for local)

## Platform-Specific Issues

### macOS Networking

**Problem:** Docker networking issues on macOS

**Solutions:**
- Use `host.docker.internal` instead of `localhost` in some contexts
- Ensure Docker Desktop networking is properly configured
- Try restarting Docker Desktop

### Linux Networking

**Problem:** Container networking issues on Linux

**Solutions:**
- Ensure Docker daemon is running: `sudo systemctl start docker`
- Check iptables rules aren't blocking connections
- Verify user is in docker group: `sudo usermod -aG docker $USER`

### Windows (WSL2) Issues

**Problem:** Performance or networking issues on Windows

**Solutions:**
- Ensure WSL2 is properly configured
- Allocate sufficient memory to WSL2
- Use WSL2 file system for better performance
- Restart WSL2: `wsl --shutdown` then restart

## Build and Development Issues

### Rust/WASM Target Missing

**Problem:** `wasm32-unknown-unknown` target not found

**Solution:**
```bash
rustup target add wasm32-unknown-unknown
```

### Substreams CLI Issues

**Problem:** Substreams commands fail

**Solutions:**
1. **Verify installation:**
   ```bash
   substreams --version
   ```

2. **Update to latest version:**
   ```bash
   # Using curl
   curl -sSf https://substreams.streamingfast.io/install | bash
   
   # Or using Homebrew
   brew install streamingfast/tap/substreams
   ```

### Node.js/NPM Issues

**Problem:** Package installation or build failures

**Solutions:**
```bash
# Clear npm cache
npm cache clean --force

# Delete node_modules and reinstall
rm -rf node_modules package-lock.json
npm install

# Use specific Node.js version
nvm use 18
```

### Foundry Issues

**Problem:** Forge commands fail

**Solutions:**
```bash
# Update Foundry
foundryup

# Clear cache
forge clean

# Reinstall dependencies
forge install
```

## Data and State Issues

### Blockchain State Problems

**Problem:** Inconsistent or corrupted blockchain state

**Solutions:**
```bash
# Reset everything (nuclear option)
docker compose down --volumes
docker system prune -f
docker compose up -d
```

### Block Production Issues

**Problem:** No new blocks being produced

**Solutions:**
- **Ethereum:** Ensure dev mode is configured with `--dev.period=1`
- **Solana:** Check test validator is running with proper configuration
- Generate transactions to trigger block production

## Performance Issues

### Slow Block Processing

**Problem:** Blocks are processed very slowly

**Solutions:**
- Increase Docker memory allocation
- Check available disk space
- Reduce block time in dev mode configuration
- Close resource-intensive applications

### High CPU Usage

**Problem:** Docker containers using excessive CPU

**Solutions:**
- Reduce logging verbosity in container configuration
- Limit container resource usage in docker-compose.yml
- Check for infinite loops in smart contracts

## Common Error Messages

### "bind: address already in use"

**Cause:** Port conflict with existing service

**Solution:** Kill the conflicting process or change ports in docker-compose.yml

### "no space left on device"

**Cause:** Insufficient disk space

**Solution:** Free up disk space or clean Docker resources:
```bash
docker system prune -a --volumes
```

### "connection reset by peer"

**Cause:** Network connectivity issues

**Solution:** Check firewall settings and restart Docker services

### "context deadline exceeded"

**Cause:** Operation timeout

**Solution:** Increase timeout values or check service health

## Getting Help

If you continue to experience issues:

1. **Check container logs:** `docker compose logs -f`
2. **Verify system requirements:** Ensure sufficient RAM, disk space, and Docker version
3. **Search existing issues:** Check GitHub repositories for similar problems
4. **Create minimal reproduction:** Isolate the issue to specific steps
5. **Gather system information:** Include OS, Docker version, and error messages when reporting issues

## Useful Commands

```bash
# System information
docker --version
docker compose version
uname -a

# Resource usage
docker stats
df -h
free -h

# Network debugging
netstat -tulpn | grep :8545
ss -tulpn | grep :9000

# Container debugging
docker compose exec <service> bash
docker inspect <container-name>
```
