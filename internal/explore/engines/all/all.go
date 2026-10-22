// Package all enables every exploration engine. Import it for side
// effects:
//
//	import _ "github.com/mehran9675/anyfile/internal/explore/engines/all"
package all

import (
	_ "github.com/mehran9675/anyfile/internal/explore/engines/archive"
	_ "github.com/mehran9675/anyfile/internal/explore/engines/binary"
	_ "github.com/mehran9675/anyfile/internal/explore/engines/blobs"
	_ "github.com/mehran9675/anyfile/internal/explore/engines/carver"
	_ "github.com/mehran9675/anyfile/internal/explore/engines/disc"
	_ "github.com/mehran9675/anyfile/internal/explore/engines/mail"
	_ "github.com/mehran9675/anyfile/internal/explore/engines/media"
	_ "github.com/mehran9675/anyfile/internal/explore/engines/ole"
	_ "github.com/mehran9675/anyfile/internal/explore/engines/pdf"
	_ "github.com/mehran9675/anyfile/internal/explore/engines/sevenzip"
	_ "github.com/mehran9675/anyfile/internal/explore/engines/xsub"
)
