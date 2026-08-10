package constant

type TaskPlatform string

const (
	TaskPlatformSuno       TaskPlatform = "suno"
	TaskPlatformMidjourney              = "mj"
)

const (
	SunoActionMusic  = "MUSIC"
	SunoActionLyrics = "LYRICS"

	TaskActionGenerate          = "generate"
	TaskActionTextGenerate      = "textGenerate"
	TaskActionFirstTailGenerate = "firstTailGenerate"
	TaskActionReferenceGenerate = "referenceGenerate"
	TaskActionRemix             = "remixGenerate"
)

const (
	TaskActionTextToVideo        = "textToVideo"
	TaskActionImageToVideo       = "imageToVideo"
	TaskActionFirstFrame         = "firstFrame"
	TaskActionFirstAndLastFrames = "firstAndLastFrames"
)

const (
	TaskVideoGenerationModeTextToVideo    = "text_to_video"
	TaskVideoGenerationModeImageToVideo   = "image_to_video"
	TaskVideoGenerationModeFirstFrame     = "first_frame"
	TaskVideoGenerationModeFirstLastFrame = "first_last_frame"
	TaskVideoGenerationModeReferenceImage = "reference_image"
)

var SunoModel2Action = map[string]string{
	"suno_music":  SunoActionMusic,
	"suno_lyrics": SunoActionLyrics,
}
