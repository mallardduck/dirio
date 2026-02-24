package clients_test

import (
	"embed"
	"fmt"
)

//go:embed scripts lib
var assets embed.FS

func GetAsset(path string) string {
	data, _ := assets.ReadFile(path)
	return string(data)
}

// awsCLITestScript returns the test script for AWS CLI
func awsCLITestScript() string {
	// Write lib files to /tmp first
	return fmt.Sprintf(`
# Write test framework libraries to /tmp
cat > /tmp/test_framework.sh << 'EOF_FRAMEWORK'
%s
EOF_FRAMEWORK

cat > /tmp/validators.sh << 'EOF_VALIDATORS'
%s
EOF_VALIDATORS

# Run the test script
%s
`, GetAsset("lib/test_framework.sh"), GetAsset("lib/validators.sh"), GetAsset("scripts/awscli.sh"))
}

// boto3TestScript returns the Python test script for boto3
func boto3TestScript() string {
	return fmt.Sprintf(`pip install --quiet boto3 requests

# Write Python test framework libraries to /tmp
mkdir -p /tmp/lib
cat > /tmp/lib/test_framework.py << 'EOF_FRAMEWORK'
%s
EOF_FRAMEWORK

cat > /tmp/lib/validators.py << 'EOF_VALIDATORS'
%s
EOF_VALIDATORS

# Add /tmp/lib to Python path and run test
cd /tmp
python3 << 'PYTHON_SCRIPT'
%s
PYTHON_SCRIPT
`, GetAsset("lib/test_framework.py"), GetAsset("lib/validators.py"), GetAsset("scripts/boto3cli.py"))
}
