# Ideal Direction: MinIO-Style IAM (Not AWS IAM)

## Executive Summary

We will **not** attempt to implement AWS IAM compatibility.
Instead, we will implement a **MinIO-style IAM and admin model**, optimized for **self-hosted S3-compatible storage** and **migration from legacy MinIO (2019–2022 era)**.

This choice prioritizes:

* Practical usability
* Migration safety
* Long-term maintainability
* Honest compatibility guarantees

over theoretical AWS parity.

---

## Rationale

### 1. AWS IAM Compatibility Is Not Realistic for Self-Hosted Systems

AWS IAM:

* Is extremely large and complex
* Uses a legacy Query API with strict XML semantics
* Requires deep SigV4 correctness
* Is expected to work with awscli, boto3, Terraform, etc.

Partial implementations are **worse than none** — they break tooling and erode trust.

No widely adopted self-hosted object store fully supports AWS IAM.

---

### 2. MinIO IAM Matches Real-World Usage

Our users:

* Are migrating from **older MinIO deployments**
* Already use **MinIO concepts** (users, policies, service accounts)
* Do not rely on `aws iam` or Terraform IAM providers

MinIO IAM is:

* Designed specifically for self-hosted storage
* Simpler and more opinionated
* Proven at scale
* Operationally understandable

---

### 3. Compatibility That Actually Matters

We target compatibility where it delivers value:

| Area                 | Compatibility   |
| -------------------- | --------------- |
| S3 Data Plane        | ✅ S3-compatible |
| MinIO IAM Semantics  | ✅ Yes           |
| `mc admin` (partial) | ✅ Yes           |
| Custom Client / SDK  | ✅ First-class   |
| AWS IAM (`aws iam`)  | ❌ No            |
| Terraform AWS IAM    | ❌ No            |

This aligns with real operator workflows.

---

## What We Will Implement

### Identity Model

* Users
* Access / secret keys
* Service accounts
* Optional groups

### Authorization

* JSON policy documents
* S3-style actions and resources
* Policy attachment to users / service accounts
* Explicit deny / allow semantics

### APIs

* MinIO Admin API (subset)
* Stable, documented REST endpoints
* Versioned API surface
* No Query API, no XML requirements

### Tooling

* Partial compatibility with `mc admin`
* Our own CLI for full functionality
* Our own SDK / client libraries

---

## What We Explicitly Will Not Implement

* AWS IAM Query API
* Path `/` multiplexing for IAM
* XML IAM responses
* `aws iam` compatibility
* Terraform AWS provider compatibility
* IAM roles, STS assume-role semantics

This avoids false expectations and long-term maintenance debt.

---

## Migration Story

* Direct migration from legacy MinIO IAM models
* Minimal retraining for operators
* Reuse of existing policy documents where possible
* Clear documentation on supported vs unsupported features

This makes the project a **drop-in operational successor**, not a conceptual reboot.

---

## Strategic Positioning

We position the system as:

> **“S3-compatible object storage with a MinIO-style control plane, designed for self-hosted environments.”**

Not:

* “AWS S3 replacement”
* “AWS IAM compatible”

This is honest, defensible, and aligned with user expectations.

---

## Long-Term Benefits

* Faster time to stability
* Lower cognitive load for users
* Smaller, testable API surface
* Freedom to evolve IAM semantics without breaking AWS tooling
* Clear upgrade path and versioning strategy

---

## Final Verdict

Choosing MinIO-style IAM is:

* Technically sound
* Operationally proven
* Aligned with your migration source
* Dramatically lower risk than AWS IAM emulation

It is the **right solution for a self-hosted S3 system in 2026**, not a compromise.
