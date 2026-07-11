<div align="center">
  <img src="docs/assets/kubesqueeze-logo.png" alt="KubeSqueeze: a Kubernetes helm squeezing a lemon" width="300">
  <h1>KubeSqueeze</h1>
  <p>
    <a href="https://github.com/elft/KubeSqueeze/actions/workflows/ci.yaml"><img src="https://github.com/elft/KubeSqueeze/actions/workflows/ci.yaml/badge.svg?branch=main" alt="Main build"></a>
    <a href="https://github.com/elft/KubeSqueeze/actions/workflows/e2e-legacy.yaml"><img src="https://github.com/elft/KubeSqueeze/actions/workflows/e2e-legacy.yaml/badge.svg" alt="Legacy Kubernetes compatibility"></a>
  </p>
</div>

## The problem

Horizontal Pod Autoscalers help individual workloads respond to demand, but their minimum replica count defaults to one. Kubernetes introduced HPA scale-to-zero support in version 1.16, but it remains an alpha feature that is disabled by default. Using it requires enabling the `HPAScaleToZero` feature gate and configuring at least one Object or External metric for each HPA scale target. That additional metrics pipeline and per-workload configuration can be valuable for demand-driven scaling, but it is a different problem from intentionally shutting down an idle environment.

[KEDA](https://keda.sh/docs/2.20/concepts/scaling-deployments/) provides event-driven scale-to-zero without relying on the Kubernetes alpha feature. It continuously monitors event sources, directly handles scaling between zero and one replica, and uses an HPA for scaling between one and many replicas. KEDA also supports time-based scaling with its [Cron scaler](https://keda.sh/docs/2.20/scalers/cron/). This is a strong fit when each workload should automatically wake in response to a queue, event, metric, or schedule, but it requires installing KEDA and defining a `ScaledObject` with triggers and sometimes authentication for each scale target.

KubeSqueeze solves a broader operational scheduling problem. When you have consciously decided that an environment, a team's environment, a region, or another group of workloads does not need to run hot 24/7, one command performs a controlled sweep across every workload selected by its namespace and filters. It records the current state, temporarily scales Deployments and StatefulSets to zero, suspends CronJobs, and later restores their original settings. It does not require a long-running autoscaling control plane or per-workload trigger configuration, but it also does not monitor demand or automatically wake a workload when an event arrives; restoration must be invoked explicitly or by an external scheduler.

KubeSqueeze does not itself scale down or terminate cluster nodes. Instead, it removes selected workload demand so a separately configured Kubernetes node autoscaler can consolidate or remove nodes when possible. It is not a replacement for a metric-driven autoscaler or HPA.

During `squeeze`, KubeSqueeze records a Deployment or StatefulSet replica count in the `kubesqueeze.io/original-replicas` annotation before scaling it to zero. For a CronJob it records the original suspend value in `kubesqueeze.io/original-suspend` before suspending it. `restore` reads that state and restores the workload. Saved annotations remain after restore so the operation can be safely repeated.

## Build

Go 1.24 or newer is required.

```sh
make build
./bin/kubesqueeze --help
```

The container image can be built with `make image IMAGE=ghcr.io/example/kubesqueeze:dev`.

## Cluster safety and selection

Every command requires an explicit kubeconfig context. KubeSqueeze never silently uses the current context. One invocation affects one context (and therefore one cluster) only. It also requires either one or more explicit namespaces or `--all-namespaces`; namespace scope is a hard boundary that selectors cannot expand.

```sh
kubesqueeze squeeze \
  --kubeconfig "$HOME/.kube/config" \
  --context kind-development \
  --namespace shop-staging \
  --include-name-regex '^(api|worker)-.*$'
```

The namespace from `--namespace` takes precedence over a namespace configured on the context. A successful selection with no matching resources is a no-op.

## Include and ignore selectors

At least one explicit `--include-*` flag is required. Repeating a flag is OR within that category; different supplied categories are ANDed. Ignore rules run after inclusion, use the same grammar, and always win. An ignore rule excludes a resource only when every supplied ignore category matches it.

| Category | Include flag | Ignore flag | Value |
| --- | --- | --- | --- |
| Name | `--include-name-regex` | `--ignore-name-regex` | Anchored RE2 regular expression |
| Namespace | `--include-namespace-regex` | `--ignore-namespace-regex` | Anchored RE2 regular expression |
| Labels | `--include-label-selector` | `--ignore-label-selector` | Kubernetes selector expression |
| Annotations | `--include-annotation-selector` | `--ignore-annotation-selector` | Kubernetes selector expression evaluated against annotations |
| Kind | `--include-kind` | `--ignore-kind` | `deployment`, `statefulset`, or `cronjob` |
| Owner | `--include-owner-regex` | `--ignore-owner-regex` | Anchored RE2 against `<apiVersion>/<kind>/<name>` |

Label and annotation selectors support equality, inequality, `in`, `notin`, existence, and non-existence. If `--include-kind` is omitted, all supported kinds are considered. Names and namespaces stay subject to the explicit namespace boundary.

For example, squeeze application workloads in staging namespaces while protecting anything labeled as critical:

```sh
kubesqueeze squeeze \
  --context production-us-east \
  --all-namespaces \
  --include-namespace-regex 'team-.*-staging' \
  --include-label-selector 'app.kubernetes.io/part-of=store' \
  --include-kind deployment \
  --include-kind statefulset \
  --ignore-label-selector 'squeeze.example.com/tier=critical'
```

Restore the same selection with the same filters:

```sh
kubesqueeze restore \
  --context production-us-east \
  --all-namespaces \
  --include-namespace-regex 'team-.*-staging' \
  --include-label-selector 'app.kubernetes.io/part-of=store' \
  --include-kind deployment \
  --include-kind statefulset \
  --ignore-label-selector 'squeeze.example.com/tier=critical'
```

Arguments are validated before mutation. Output is deterministic JSON on stdout; errors are JSON on stderr with a nonzero exit status. Resource records are sorted by namespace, kind, and name.

## Supported resources and Kubernetes versions

- Deployments and StatefulSets use `spec.replicas`.
- CronJobs use `spec.suspend`; KubeSqueeze discovers either `batch/v1` or the legacy `batch/v1beta1` API at runtime.
- ReplicaSets, DaemonSets, Jobs, custom resources, and controller-owned children are not modified in v1.

CI tests Kubernetes 1.20 and the current pinned kind node image on pull requests. Kubernetes 1.15 is end-of-life and runs as a scheduled, non-blocking compatibility lane. See [the compatibility workflow](.github/workflows/e2e-legacy.yaml) for the exact image digests. Supporting an old API does not restore upstream security support for an end-of-life Kubernetes cluster.

## Scheduling

KubeSqueeze has no internal scheduler. [`examples/github-actions/scheduled-external-cluster.yaml`](examples/github-actions/scheduled-external-cluster.yaml) shows separate GitHub Actions schedules for squeeze and restore against an external cluster. The example is deliberately outside `.github/workflows`, so copying this repository cannot activate cluster mutations.

Before adapting the example:

1. Put a base64-encoded kubeconfig in the protected environment secret `KUBECONFIG_B64`.
2. Put the only permitted context name in the environment variable `KUBESQUEEZE_CONTEXT`.
3. Require reviewers on the GitHub environment used by scheduled jobs.
4. Bind the kubeconfig identity to least-privilege RBAC such as [`examples/rbac.yaml`](examples/rbac.yaml).
5. Pin the downloaded KubeSqueeze release and verify its checksum.

The example schedules are UTC. GitHub schedule delivery can be delayed, so do not treat it as an exact wall-clock guarantee.

## Development

```sh
make test
make check
make e2e KIND_NODE_IMAGE='kindest/node:v1.35.0@sha256:452d707d4862f52530247495d180205e029056831160e22870e37e3f6c1ac31f'
```

The E2E harness creates an isolated kind cluster, applies selected/ignored/control fixtures, runs squeeze and restore, and verifies exact normalized JSON plus cluster state. `PRESERVE_CLUSTER=1` keeps the cluster for troubleshooting.

## License

KubeSqueeze is available under the [MIT License](LICENSE).
