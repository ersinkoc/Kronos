# Kronos Grafana Dashboards

This directory contains importable Grafana dashboard examples for Kronos
Prometheus metrics.

## Overview Dashboard

Import `kronos-overview.json` into Grafana, then select the Prometheus data
source that scrapes the Kronos control plane `/metrics` endpoint.

The dashboard focuses on the implemented production metrics:

- control-plane build and uptime
- agent health and schedulable capacity
- active jobs by operation and agent
- backup freshness, volume, chunks, and protected backup counts
- resource inventory and schedule pause state
- token cleanup and auth rate-limit signals

Tune panel thresholds to match the deployment's backup windows, agent capacity,
and token rotation cadence before using the dashboard as an on-call signal.
