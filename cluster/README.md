# Cluster-wide configuration

Everything under `cluster/` governs the whole node, not just
pet-messenger, and is applied by hand under the cluster admin's
kubeconfig — **never** by any single project's CI. A project's own
deploy pipeline must not be able to edit the ceiling that constrains it;
that would defeat the point of having one.

## Why this node needs quotas at all

Found via a real incident on a different shared box: one project with no
`resources.requests` set on any container ate almost all the node's RAM,
and an unrelated project's pods started failing with no indication why —
the error surfaced as "node ran out of memory," not as anything pointing
at the actual cause. `deploy/kubernetes/base/*.yaml` now sets explicit
`resources` on every container (see that directory), and the
`ResourceQuota`/`LimitRange` pair here backstops that from outside the
namespace, so a future change inside pet-messenger's own manifests can't
silently regress the same way.

## Node-wide arithmetic (4096Mi node, k3s)

```
4096Mi  node total
- 512Mi  k3s control plane + Traefik + CoreDNS + metrics-server +
         kubelet/containerd + cert-manager (~50-100Mi of this reserve)
= 3584Mi available to workload namespaces
- 1792Mi pet-messenger quota (limits.memory) — see quotas/pet-messenger.yaml
= 1792Mi left for the other namespace sharing this node
```

Per-workload sizing that the pet-messenger numbers above come from is in
`../deploy/kubernetes/README.md` and the design spec
`../docs/superpowers/specs/2026-10-12-k8s-shared-server-deploy-cicd-design.md`.

**The 1792Mi left over is arithmetic, not a measured fact about the other
project.** Its real memory need was unknown when this was written — see
`quotas/other-project.yaml`, which is a template, not a verified number.
If that project's administrator sizes its quota at or under 1792Mi,
everything fits with some margin left over. If it needs more, something
has to give — tighter limits somewhere, fewer replicas, or more RAM on
the node — and that decision belongs to whoever runs that project, not to
this file.

## Applying

```bash
kubectl apply -f cluster/quotas/pet-messenger.yaml
# After confirming the other project's real namespace name and need:
kubectl apply -f cluster/quotas/other-project.yaml
```

See `cert-manager/README.md` in this directory for the cluster's shared
TLS infrastructure.
