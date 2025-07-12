package utils

func RemoveFromSlice(slice []any, item any) []any {
	for i, v := range slice {
		if v == item {
			slice = append(slice[:i], slice[i+1:]...)
			return slice
		}
	}
	return slice
}
