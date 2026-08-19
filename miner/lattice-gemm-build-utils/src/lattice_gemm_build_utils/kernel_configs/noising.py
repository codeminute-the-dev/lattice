"""
Noising kernel configuration schemas.

Defines configurations for noising A and noising B kernels used in the
noisy GEMM pipeline.
"""

from typing import Literal

from pydantic import BaseModel, Field, field_validator

DenoiseType = Literal["fp16", "int32"]


def parse_denoise_type(s: str) -> DenoiseType:
    """Parse a denoise type string, normalizing 'int' to 'int32'."""
    out = s.lower()
    if out == "int":
        out = "int32"
    if out not in ("fp16", "int32"):
        raise ValueError(f"Invalid denoise type: {s}. Must be 'fp16' or 'int32'.")
    return out  # type: ignore


class NoisingAKernelConfig(BaseModel):
    """
    Build-time configuration for noising A kernel codegen.

    Used by setup.py and lattice-gemm-build-utils to generate kernel instantiations.
    """

    model_config = {"frozen": True, "extra": "forbid"}

    # Tile dimensions (matches JSON field names)
    tile_size_m: int = Field(ge=16, le=256, description="Tile size in M dimension")
    tile_size_k: int = Field(ge=32, le=256, description="Tile size in K dimension")

    # Low-rank dimension
    R: int = Field(ge=16, le=256, description="Low-rank dimension for noise matrices")

    # Pipeline configuration
    pipeline_stages: int = Field(default=2, ge=1, le=4, description="Number of pipeline stages")

    # Denoise output type (matches JSON field name)
    AxEBL_type: DenoiseType = Field(default="fp16")

    @field_validator("AxEBL_type", mode="before")
    @classmethod
    def parse_denoise_type(cls, v):
        """Parse and normalize denoise type."""
        if isinstance(v, str):
            return parse_denoise_type(v)
        return v


class NoisingBKernelConfig(BaseModel):
    """
    Build-time configuration for noising B kernel codegen.

    Used by setup.py and lattice-gemm-build-utils to generate kernel instantiations.
    """

    model_config = {"frozen": True, "extra": "forbid"}

    # Tile dimensions (matches JSON field names)
    tile_size_n: int = Field(ge=16, le=256, description="Tile size in N dimension")
    tile_size_k: int = Field(ge=32, le=256, description="Tile size in K dimension")

    # Low-rank dimension
    R: int = Field(ge=16, le=256, description="Low-rank dimension for noise matrices")

    # Pipeline configuration
    pipeline_stages: int = Field(default=2, ge=1, le=4, description="Number of pipeline stages")

    # Denoise output type (matches JSON field name)
    EARxBpEB_type: DenoiseType = Field(default="fp16")

    @field_validator("EARxBpEB_type", mode="before")
    @classmethod
    def parse_denoise_type(cls, v):
        """Parse and normalize denoise type."""
        if isinstance(v, str):
            return parse_denoise_type(v)
        return v
