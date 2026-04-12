package extractor

var extractConfig = `{
		"output_format": "markdown",
		"results_format": "element_based",
		"include_document_structure": true,
		"ocr": {
			"backend": "tesseract",
			"language": "eng+deu",
			"tesseract_config": {
				"psm": 3,
				"oem": 3,
				"min_confidence": 0.0,
				"output_format": "text",
				"enable_table_detection": true,
				"table_min_confidence": 0.5,
				"table_column_threshold": 50,
				"table_row_threshold_ratio": 0.5,
				"use_cache": true,
				"preprocessing": {
					"target_dpi": 300,
					"auto_rotate": true,
					"deskew": true,
					"denoise": true,
					"contrast_enhance": true,
					"binarization_method": "otsu",
					"invert_colors": false
				}
			}
	},
	"pdf_options": {
		"extract_metadata": true
	},
	"chunking": {
		"max_characters": 500,
		"overlap": 100,
		"chunker_type": "markdown",
		"sizing": {
			"type": "tokenizer",
			"model": "Xenova/gpt-4o"
		}
	}
}`
