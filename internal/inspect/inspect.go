package inspect

func Stats(inspector Inspector) (string, error) {
	result, err := inspector.Stats()
	if err != nil {
		return "", err
	}

	return result, nil
}

func Dump(inspector Inspector) (string, error) {
	result, err := inspector.Dump()
	if err != nil {
		return "", err
	}
	return result, nil
}
