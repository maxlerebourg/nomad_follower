# nomad_follower
Log forwarder for aggregating allocation logs from nomad worker agents.

## Running the application 
Run the application on each worker in a nomad cluster.
```
docker pull sofixa/nomad_follower:latest
docker run -v log_folder:/log -e LOG_META="logging" "-e LOG_FILE="/log/nomad-forwarder.log" sofixa/nomad_follower:latest
```

Example Nomad job file available [here](./nomad_follower.nomad), with an example job outputting random logs [here](./example.nomad).

nomad_follower:
 - will follow allocations based on task metadata - set the meta key matching LOG_TAG (default: nomad-follower) to true in your task's meta block to enable logging for that specific task.
 - will stop following completed allocations and will start following new allocations as they become available. 
 - can be deployed with nomad in a system task group along with a log collector. The aggregate log file can then be shared with the log collector by writing the aggregate log file into the shared allocation folder. 
 - formats log entries as json formatted logs. It will convert string formatted logs to json formatted logs by passing the log entry in the ```message``` key. 
 - adds a ```service_name``` key that contains the listed service names for a task.

## Configuration

### Environment Variables

- LOG_TAG (default: nomad-follower): Meta key used to identify tasks to follow.
- LOG_ENABLED_BY_DEFAULT (default: false): If true, all tasks are logged unless explicitly disabled.
- LOCAL_NODE_ONLY (default: true): If true, only follow allocations on the local node. Set to false to follow all allocations across the cluster.
- LOG_FILE (default: nomad.log): Path to the aggregate log file.
- SAVE_FILE (default: nomad-follower.json): Path to the state save file for crash recovery.
- LOG_LEVEL (default: INFO): Verbosity level (TRACE, DEBUG, INFO, ERROR).

### Task Filtering Examples

**Using Task Metadata:**
```hcl
task "my-app" {
  meta {
    nomad-follower = "true"  # Enable logging for this task
  }
}
```

Using nomad_follower prevents the cluster operator from having to run a log collector in every task group for every task on a worker while still allowing nomad to handle the logs for each allocation. 

# License and attribution

This project is a fork of [this fork](https://github.com/sas1024/nomad_follower) of this original [project](https://github.com/adragoset/nomad_follower).
Everything within is licensed under [GPL v3](./LICENSE)
