# Enable the server
server {
  enabled = true
  bootstrap_expect = 1
  raft_protocol = 3
}

# Increase log verbosity
log_level = "DEBUG"

# Setup data dir
data_dir = "/opt/nomad/server1"


# Give the agent a unique name. Defaults to hostname
name = "server1"

