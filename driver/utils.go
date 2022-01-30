package driver

import (
	"encoding/base64"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/apimachinery/pkg/util/sets"
	"regexp"
	"strconv"
	"strings"
)

func seenIndexesToBase64String(ints []int) string {
	result := ""
	for _, elem := range ints {
		result += strconv.Itoa(elem) + ","
	}

	result = strings.TrimRight(result, ",")
	return base64.StdEncoding.EncodeToString([]byte(result))
}

func base64SeenIndexesToMap(value string) (*sets.Int, error) {
	result := sets.NewInt()

	s, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}

	for _, strInt := range strings.Split(string(s), ",") {
		intValue, err := strconv.Atoi(strInt)
		if err != nil {
			return nil, err
		}
		result.Insert(intValue)
	}

	return nil, err
}

var cleanRegex = regexp.MustCompile(`([^a-z0-9A-Z]*)`)

func cleanISCSIName(name string) string {
	return cleanRegex.ReplaceAllString(strings.ToLower(name), "")
}

func extractStorage(capRange *csi.CapacityRange) (int64, error) {
	if capRange == nil {
		return defaultVolumeSizeInBytes, nil
	}

	requiredBytes := capRange.GetRequiredBytes()
	requiredSet := 0 < requiredBytes
	limitBytes := capRange.GetLimitBytes()
	limitSet := 0 < limitBytes

	if !requiredSet && !limitSet {
		return defaultVolumeSizeInBytes, nil
	}

	if requiredSet && limitSet && limitBytes < requiredBytes {
		return 0, fmt.Errorf("limit (%v) can not be less than required (%v) size", formatBytes(limitBytes), formatBytes(requiredBytes))
	}

	if requiredSet && !limitSet && requiredBytes < minimumVolumeSizeInBytes {
		return 0, fmt.Errorf("required (%v) can not be less than minimum supported volume size (%v)", formatBytes(requiredBytes), formatBytes(minimumVolumeSizeInBytes))
	}

	if limitSet && limitBytes < minimumVolumeSizeInBytes {
		return 0, fmt.Errorf("limit (%v) can not be less than minimum supported volume size (%v)", formatBytes(limitBytes), formatBytes(minimumVolumeSizeInBytes))
	}

	if limitSet && limitBytes % (1 * giB) != 0 {
		return 0, fmt.Errorf("limit (%v) must be a multiple of 1GB", limitBytes)
	}

	if requiredSet && requiredBytes > maximumVolumeSizeInBytes {
		return 0, fmt.Errorf("required (%v) can not exceed maximum supported volume size (%v)", formatBytes(requiredBytes), formatBytes(maximumVolumeSizeInBytes))
	}

	if requiredSet && requiredBytes % (1 * giB) != 0 {
		return 0, fmt.Errorf("required (%v) must be a multiple of 1GB", requiredBytes)
	}

	if !requiredSet && limitSet && limitBytes > maximumVolumeSizeInBytes {
		return 0, fmt.Errorf("limit (%v) can not exceed maximum supported volume size (%v)", formatBytes(limitBytes), formatBytes(maximumVolumeSizeInBytes))
	}

	if requiredSet && limitSet && requiredBytes == limitBytes {
		return requiredBytes, nil
	}

	if requiredSet {
		return requiredBytes, nil
	}

	if limitSet {
		return limitBytes, nil
	}

	return defaultVolumeSizeInBytes, nil
}

func formatBytes(inputBytes int64) string {
	output := float64(inputBytes)
	unit := ""

	switch {
	case inputBytes >= tiB:
		output = output / tiB
		unit = "Ti"
	case inputBytes >= giB:
		output = output / giB
		unit = "Gi"
	case inputBytes >= miB:
		output = output / miB
		unit = "Mi"
	case inputBytes >= kiB:
		output = output / kiB
		unit = "Ki"
	case inputBytes == 0:
		return "0"
	}

	result := strconv.FormatFloat(output, 'f', 1, 64)
	result = strings.TrimSuffix(result, ".0")
	return result + unit
}