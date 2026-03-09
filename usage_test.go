package config_test

import (
	"os"
	"testing"

	"github.com/infracost/config"
	"github.com/infracost/go-proto/pkg/rat"
	protousage "github.com/infracost/proto/gen/go/infracost/usage"

	"github.com/stretchr/testify/require"
)

func Test_ParseExample(t *testing.T) {
	f, err := os.Open("./infracost-usage-example.yml")
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	output, err := config.LoadUsageYAML(f, nil)
	require.NoError(t, err)

	require.NotNil(t, output.ByResourceType, "resource type config should not be nil")
	require.NotNil(t, output.ByAddress, "address config should not be nil")

	lambdaConfig, ok := output.ByResourceType["aws_lambda_function"]
	require.True(t, ok, "expected aws_lambda_function in resource type config")

	require.NotNil(t, lambdaConfig.Items, "aws_lambda_function should have config entries")
	monthlyRequests, ok := lambdaConfig.Items["monthly_requests"]
	require.True(t, ok, "expected monthly_requests in aws_lambda_function config")
	require.NotNil(t, monthlyRequests, "monthly_requests config should not be nil")
	require.NotNil(t, monthlyRequests.Value, "monthly_requests value should not be nil")
	numValue, ok := monthlyRequests.Value.(*protousage.UsageValue_NumberValue)
	require.True(t, ok, "monthly_requests value should be a number")
	require.Equal(t, 100000, rat.FromProto(numValue.NumberValue).Int(), "monthly_requests value should be 5000000")
	requestDurationMs, ok := lambdaConfig.Items["request_duration_ms"]
	require.True(t, ok, "expected request_duration_ms in aws_lambda_function config")
	require.NotNil(t, requestDurationMs, "request_duration_ms config should not be nil")
	require.NotNil(t, requestDurationMs.Value, "request_duration_ms value should not be nil")
	numValue, ok = requestDurationMs.Value.(*protousage.UsageValue_NumberValue)
	require.True(t, ok, "request_duration_ms value should be a number")
	require.Equal(t, 500, rat.FromProto(numValue.NumberValue).Int(), "request_duration_ms value should be 300")

	acmpcaCertAuthority, ok := output.ByAddress["aws_acmpca_certificate_authority.my_private_ca"]
	require.True(t, ok, "expected aws_acmpca_certificate_authority.my_private_ca in address config")
	require.NotNil(t, acmpcaCertAuthority.Items, "aws_acmpca_certificate_authority.my_private_ca should have config entries")
	monthlyRequests, ok = acmpcaCertAuthority.Items["monthly_requests"]
	require.True(t, ok, "expected monthly_requests in aws_acmpca_certificate_authority.my_private_ca config")
	require.NotNil(t, monthlyRequests, "monthly_requests config should not be nil")
	require.NotNil(t, monthlyRequests.Value, "monthly_requests value should not be nil")
	numValue, ok = monthlyRequests.Value.(*protousage.UsageValue_NumberValue)
	require.True(t, ok, "monthly_requests value should be a number")
	require.Equal(t, 20000, rat.FromProto(numValue.NumberValue).Int(), "monthly_requests value should be 2000")
}
