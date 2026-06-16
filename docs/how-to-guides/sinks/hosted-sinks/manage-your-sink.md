# Managing Your Hosted Sink

Once a sink is deployed, the sink detail page gives you visibility into its runtime state and controls to manage its lifecycle.

## Sink Detail Page

Navigate to **Sinks** and click a sink to open its detail page. The page has three main sections: the runtime summary, the pod list, and the deployment event log.

### Runtime Summary

The summary strip shows four metrics at a glance:

| Metric | Meaning |
|---|---|
| **Replicas** | `ready / total` — how many replicas are ready vs. requested. |
| **Instances** | Number of active pods. |
| **Restarts** | Total container restarts across all pods. Elevated counts indicate instability. |
| **Output module** | The Substreams module being executed, and the package source URL. |

### Pod List

Each pod row shows:

- **Status badge** — the execution state of that pod (see [Execution States](#execution-states) below).
- **Current block / Head block** — how far behind the chain head the pod is.
- **Head block time drift** — seconds behind real-time. Near zero means the pod is live.
- **Restart count** — restarts for this individual pod.
- **Logs** — click to view the latest container logs for that pod.

### Deployment Events

The events log records lifecycle changes to the deployment (e.g., created, config updated, stopped). Use it to audit changes or troubleshoot issues.

## Status Reference

### Deployment Status

The status badge in the page header reflects the overall deployment state.

| Status | Meaning |
|---|---|
| **Deploying** | Pods are starting up; replicas requested but not yet ready. |
| **Deployed** | All requested replicas are running. |
| **Stopped** | Replica count is set to 0; no pods are running. |
| **Error** | The deployment encountered a hard failure. |
| **Deleting** | The deployment is being torn down. |

### Execution States

Pod-level execution states show how far along the indexing process a running pod is.

| State | Meaning |
|---|---|
| **Initiating** | Pod started; sink process is initializing. |
| **Catching up** | Actively processing historical blocks behind the chain head. |
| **Live** | Caught up to the chain head; processing new blocks as they arrive. |
| **Failing** | Pod encountered an error during processing. Check logs. |

{% hint style="info" %}
A `CrashLoopBackOff` banner appears at the top of the detail page when one or more pods are repeatedly crashing. This usually indicates a database connection issue or a schema mismatch. Check pod logs for the root cause.
{% endhint %}

## Editing a Sink

Click **Edit** in the top-right toolbar to open the edit modal. Changes take effect after you save — the sink process restarts automatically.

### Deployment tab

Update the Substreams package source (URL, Substreams.dev ID, or upload a new binary) and the execution config (start block, stop block, output module, filters, parameters).

### Output tab

Update the database connection details (host, port, credentials, SSL mode). Useful when your database credentials rotate or you migrate to a new host.

## Stopping and Starting a Sink

Click **Stop** to scale the deployment to zero replicas. The sink process halts and no data is written to your database, but the deployment config is preserved.

To restart, click **Start**. The sink resumes from its last cursor position — no data is re-indexed from scratch unless you explicitly reset (see below).

## Resetting a Sink

To re-index from a specific block, edit the sink and set a new **Start block** with the **Restart from scratch** option enabled. This clears the cursor and drops or truncates the existing data depending on the reset mode chosen.

{% hint style="warning" %}
Resetting is irreversible. All previously indexed data in the target schema will be removed.
{% endhint %}

## Deleting a Sink

Click **Delete** in the toolbar and confirm. The deployment and all associated pods are permanently removed. Your database and its data are not affected — only the hosted sink process is deleted.
