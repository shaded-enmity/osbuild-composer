package image

import (
	"fmt"
	"math/rand"

	"github.com/osbuild/osbuild-composer/internal/artifact"
	"github.com/osbuild/osbuild-composer/internal/common"
	"github.com/osbuild/osbuild-composer/internal/disk"
	"github.com/osbuild/osbuild-composer/internal/environment"
	"github.com/osbuild/osbuild-composer/internal/manifest"
	"github.com/osbuild/osbuild-composer/internal/osbuild"
	"github.com/osbuild/osbuild-composer/internal/platform"
	"github.com/osbuild/osbuild-composer/internal/rpmmd"
	"github.com/osbuild/osbuild-composer/internal/runner"
	"github.com/osbuild/osbuild-composer/internal/workload"
)

type LiveImage struct {
	Base
	Platform         platform.Platform
	PartitionTable   *disk.PartitionTable
	OSCustomizations manifest.OSCustomizations
	Environment      environment.Environment
	Workload         workload.Workload
	Filename         string
	Compression      string
	ForceSize        *bool
	PartTool         osbuild.PartTool

	NoBLS     bool
	OSProduct string
	OSVersion string
	OSNick    string
}

func NewLiveImage() *LiveImage {
	return &LiveImage{
		Base:     NewBase("live-image"),
		PartTool: osbuild.PTSfdisk,
	}
}

func (img *LiveImage) InstantiateManifest(m *manifest.Manifest,
	repos []rpmmd.RepoConfig,
	runner runner.Runner,
	rng *rand.Rand) (*artifact.Artifact, error) {
	buildPipeline := manifest.NewBuild(m, runner, repos)
	buildPipeline.Checkpoint()

	osPipeline := manifest.NewOS(m, buildPipeline, img.Platform, repos)
	osPipeline.PartitionTable = img.PartitionTable
	osPipeline.OSCustomizations = img.OSCustomizations
	osPipeline.Environment = img.Environment
	osPipeline.Workload = img.Workload
	osPipeline.NoBLS = img.NoBLS
	osPipeline.OSProduct = img.OSProduct
	osPipeline.OSVersion = img.OSVersion
	osPipeline.OSNick = img.OSNick

	imagePipeline := manifest.NewRawImage(m, buildPipeline, osPipeline)
	imagePipeline.PartTool = img.PartTool

	var artifact *artifact.Artifact
	var artifactPipeline manifest.Pipeline
	switch img.Platform.GetImageFormat() {
	case platform.FORMAT_RAW:
		if img.Compression == "" {
			imagePipeline.Filename = img.Filename
		}
		artifactPipeline = imagePipeline
		artifact = imagePipeline.Export()
	case platform.FORMAT_QCOW2:
		qcow2Pipeline := manifest.NewQCOW2(m, buildPipeline, imagePipeline)
		if img.Compression == "" {
			qcow2Pipeline.Filename = img.Filename
		}
		qcow2Pipeline.Compat = img.Platform.GetQCOW2Compat()
		artifactPipeline = qcow2Pipeline
		artifact = qcow2Pipeline.Export()
	case platform.FORMAT_VHD:
		vpcPipeline := manifest.NewVPC(m, buildPipeline, imagePipeline)
		if img.Compression == "" {
			vpcPipeline.Filename = img.Filename
		}
		vpcPipeline.ForceSize = img.ForceSize
		artifactPipeline = vpcPipeline
		artifact = vpcPipeline.Export()
	case platform.FORMAT_VMDK:
		vmdkPipeline := manifest.NewVMDK(m, buildPipeline, imagePipeline)
		if img.Compression == "" {
			vmdkPipeline.Filename = img.Filename
		}
		artifactPipeline = vmdkPipeline
		artifact = vmdkPipeline.Export()
	case platform.FORMAT_GCE:
		// NOTE(akoutsou): temporary workaround; filename required for GCP
		// TODO: define internal raw filename on image type
		imagePipeline.Filename = "disk.raw"
		archivePipeline := manifest.NewTar(m, buildPipeline, imagePipeline, "archive")
		archivePipeline.Format = osbuild.TarArchiveFormatOldgnu
		archivePipeline.RootNode = osbuild.TarRootNodeOmit
		// these are required to successfully import the image to GCP
		archivePipeline.ACLs = common.ToPtr(false)
		archivePipeline.SELinux = common.ToPtr(false)
		archivePipeline.Xattrs = common.ToPtr(false)
		archivePipeline.Filename = img.Filename // filename extension will determine compression
	default:
		panic("invalid image format for image kind")
	}

	switch img.Compression {
	case "xz":
		xzPipeline := manifest.NewXZ(m, buildPipeline, artifactPipeline)
		xzPipeline.Filename = img.Filename
		artifact = xzPipeline.Export()
	case "":
		// do nothing
	default:
		// panic on unknown strings
		panic(fmt.Sprintf("unsupported compression type %q", img.Compression))
	}

	return artifact, nil
}
