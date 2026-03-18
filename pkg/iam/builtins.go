package iam

import "slices"

// BuiltinPolicyNames lists the names of the built-in MinIO-compatible policies
// seeded at startup. Order matches MinIO's DefaultPolicies list.
var BuiltinPolicyNames = []string{
	"readwrite",
	"readonly",
	"writeonly",
	"diagnostics",
	"consoleAdmin",
}

// IsBuiltinPolicy reports whether name is a built-in (system) policy.
func IsBuiltinPolicy(name string) bool {
	return slices.Contains(BuiltinPolicyNames, name)
}

// BuiltinPolicyDocument returns the PolicyDocument for a built-in policy by name.
// Returns nil if name is not a built-in policy name.
func BuiltinPolicyDocument(name string) *PolicyDocument {
	return builtinPolicyDocuments[name]
}

// builtinPolicyDocuments holds the IAM policy documents for each built-in policy,
// matching MinIO's DefaultPolicies (github.com/minio/pkg/blob/main/policy/constants.go).
var builtinPolicyDocuments = map[string]*PolicyDocument{
	// readwrite - full S3 access (AllActions on *)
	"readwrite": {
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Effect:   "Allow",
				Action:   "s3:*",
				Resource: "arn:aws:s3:::*",
			},
		},
	},

	// readonly - GetBucketLocation + GetObject only; denies user admin actions
	"readonly": {
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Effect: "Allow",
				Action: []string{
					"s3:GetBucketLocation",
					"s3:GetObject",
				},
				Resource: "arn:aws:s3:::*",
			},
			{
				Effect:   "Deny",
				Action:   "admin:CreateUser",
				Resource: "arn:aws:s3:::*",
			},
		},
	},

	// writeonly - PutObject only
	"writeonly": {
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Effect:   "Allow",
				Action:   "s3:PutObject",
				Resource: "arn:aws:s3:::*",
			},
		},
	},

	// diagnostics - admin diagnostic actions
	"diagnostics": {
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Effect: "Allow",
				Action: []string{
					"admin:Profiling",
					"admin:Trace",
					"admin:ConsoleLog",
					"admin:ServerInfo",
					"admin:TopLocksInfo",
					"admin:HealthInfo",
					"admin:BandwidthMonitor",
					"admin:Prometheus",
				},
				Resource: "arn:aws:s3:::*",
			},
		},
	},

	// consoleAdmin - full admin + S3 access
	"consoleAdmin": {
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Effect:   "Allow",
				Action:   "admin:*",
				Resource: "arn:aws:s3:::*",
			},
			{
				Effect:   "Allow",
				Action:   "s3:*",
				Resource: "arn:aws:s3:::*",
			},
		},
	},
}
