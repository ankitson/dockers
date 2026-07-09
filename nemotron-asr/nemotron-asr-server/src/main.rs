use axum::{
    extract::{Multipart, State},
    http::StatusCode,
    response::Json,
    routing::{get, post},
    Router,
};
use parakeet_rs::{Nemotron, Parakeet, Transcriber};
use serde::Serialize;
use std::path::PathBuf;
use std::process::Command;
use std::sync::{Arc, Mutex};
use tempfile::NamedTempFile;

enum LoadedModel {
    Nemotron(Nemotron),
    ParakeetCtc(Parakeet),
}

struct ModelEntry {
    model: Mutex<LoadedModel>,
    id_prefix: String,
}

struct AppState {
    models: Vec<ModelEntry>,
}

#[derive(Serialize)]
struct HealthResponse {
    status: String,
    models: Vec<String>,
    available_prefixes: Vec<String>,
}

#[derive(Serialize)]
struct TranscriptionResponse {
    text: String,
}

fn detect_model_type(path: &std::path::Path) -> Result<&'static str, String> {
    let has_encoder = path.join("encoder.onnx").exists();
    let has_model = path.join("model.onnx").exists();
    let has_decoder_joint = path.join("decoder_joint.onnx").exists();

    if has_encoder && has_decoder_joint {
        Ok("nemotron")
    } else if has_model {
        Ok("parakeet-ctc")
    } else {
        Err(format!(
            "Unknown model type in {}. Expected encoder.onnx+decoder_joint.onnx (nemotron) or model.onnx (parakeet-ctc)",
            path.display()
        ))
    }
}

fn load_model(path: &std::path::Path) -> Result<LoadedModel, String> {
    let model_type = detect_model_type(path)?;
    match model_type {
        "nemotron" => {
            let m = Nemotron::from_pretrained(path, None)
                .map_err(|e| format!("Nemotron load: {e}"))?;
            Ok(LoadedModel::Nemotron(m))
        }
        "parakeet-ctc" => {
            let m = Parakeet::from_pretrained(path, None)
                .map_err(|e| format!("Parakeet CTC load: {e}"))?;
            Ok(LoadedModel::ParakeetCtc(m))
        }
        _ => Err("unreachable".into()),
    }
}

fn transcribe_audio_samples(model: &mut LoadedModel, samples: &[f32], sample_rate: u32, channels: u16) -> Result<String, String> {
    match model {
        LoadedModel::Nemotron(m) => {
            m.reset();
            m.transcribe_audio(samples).map_err(|e| format!("Nemotron: {e}"))
        }
        LoadedModel::ParakeetCtc(m) => {
            let result = m.transcribe_samples(samples.to_vec(), sample_rate, channels, None)
                .map_err(|e| format!("Parakeet CTC: {e}"))?;
            Ok(result.text)
        }
    }
}

fn find_model<'a>(models: &'a [ModelEntry], model_id: &str) -> Option<&'a ModelEntry> {
    // Match if the model_id (after last /) starts with the dir name, or exact match
    let suffix = model_id.rsplit('/').next().unwrap_or(model_id);
    for entry in models {
        if model_id == entry.id_prefix || suffix == entry.id_prefix || suffix.starts_with(&entry.id_prefix) {
            return Some(entry);
        }
    }
    None
}

async fn health(state: State<Arc<AppState>>) -> Json<HealthResponse> {
    let available_prefixes: Vec<String> = state.models.iter().map(|m| m.id_prefix.clone()).collect();
    Json(HealthResponse {
        status: "ok".to_string(),
        models: available_prefixes.iter().map(|p| format!("contains '{}'", p)).collect(),
        available_prefixes,
    })
}

async fn transcriptions(
    state: State<Arc<AppState>>,
    mut multipart: Multipart,
) -> Result<Json<TranscriptionResponse>, (StatusCode, String)> {
    let mut audio_data: Vec<u8> = Vec::new();
    let mut model_id = String::new();

    while let Some(field) = multipart
        .next_field()
        .await
        .map_err(|e| (StatusCode::BAD_REQUEST, format!("multipart: {e}")))?
    {
        match field.name() {
            Some("file") => {
                audio_data = field
                    .bytes()
                    .await
                    .map_err(|e| (StatusCode::BAD_REQUEST, format!("read file: {e}")))?
                    .to_vec();
            }
            Some("model") => {
                model_id = field
                    .text()
                    .await
                    .map_err(|e| (StatusCode::BAD_REQUEST, format!("read model: {e}")))?;
            }
            _ => {}
        }
    }

    if audio_data.is_empty() {
        return Err((StatusCode::BAD_REQUEST, "empty audio file".into()));
    }

    let entry = find_model(&state.models, &model_id).ok_or_else(|| {
        let available: Vec<&str> = state.models.iter().map(|m| m.id_prefix.as_str()).collect();
        (
            StatusCode::BAD_REQUEST,
            format!("unknown model '{model_id}'. Available: {:?}", available),
        )
    })?;

    let input_file = NamedTempFile::with_suffix(".in")
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("tempfile: {e}")))?;
    let output_wav = NamedTempFile::with_suffix(".wav")
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("tempfile: {e}")))?;

    std::fs::write(input_file.path(), &audio_data)
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("write: {e}")))?;

    let status = Command::new("ffmpeg")
        .args([
            "-y",
            "-i",
            input_file.path().to_str().unwrap(),
            "-ac",
            "1",
            "-ar",
            "16000",
            "-f",
            "wav",
            output_wav.path().to_str().unwrap(),
        ])
        .output()
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("ffmpeg: {e}")))?;

    if !status.status.success() {
        let stderr = String::from_utf8_lossy(&status.stderr);
        return Err((
            StatusCode::BAD_REQUEST,
            format!("audio conversion failed: {stderr}"),
        ));
    }

    let reader = hound::WavReader::open(output_wav.path())
        .map_err(|e| (StatusCode::BAD_REQUEST, format!("wav: {e}")))?;

    let spec = reader.spec();
    let samples: Vec<f32> = match spec.sample_format {
        hound::SampleFormat::Float => reader.into_samples::<f32>().filter_map(|s| s.ok()).collect(),
        hound::SampleFormat::Int => {
            let max = (1i32 << (spec.bits_per_sample - 1)) as f32;
            reader
                .into_samples::<i32>()
                .filter_map(|s| s.ok())
                .map(|s| s as f32 / max)
                .collect()
        }
    };

    let mut model = entry.model.lock().unwrap();
    let text = transcribe_audio_samples(&mut model, &samples, spec.sample_rate as u32, spec.channels as u16)
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("transcribe: {e}")))?;

    Ok(Json(TranscriptionResponse { text }))
}

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt::init();

    let model_dirs_env = std::env::var("MODEL_DIRS")
        .unwrap_or_else(|_| std::env::var("NEMOTRON_MODEL_PATH").unwrap_or_else(|_| "/models/nemotron".to_string()));

    let dirs: Vec<PathBuf> = model_dirs_env.split(',').map(|s| s.trim().into()).collect();

    let mut models = Vec::new();
    for dir in &dirs {
        if !dir.exists() {
            tracing::warn!("Model directory not found: {}", dir.display());
            continue;
        }
                let dirname = dir.file_name().unwrap_or_default().to_string_lossy().to_string();
                tracing::info!("Loading model '{}' from: {}", dirname, dir.display());
        match load_model(dir) {
            Ok(model) => {
                models.push(ModelEntry {
                    model: Mutex::new(model),
                    id_prefix: dirname.clone(),
                });
                tracing::info!("  -> loaded");
            }
            Err(e) => {
                tracing::error!("  -> failed: {e}");
            }
        }
    }

    if models.is_empty() {
        tracing::error!("No models loaded. Exiting.");
        std::process::exit(1);
    }

    for m in &models {
        tracing::info!("Available model: id_prefix='{}'", m.id_prefix);
    }

    let state = Arc::new(AppState { models });

    let app = Router::new()
        .route("/health", get(health))
        .route("/v1/audio/transcriptions", post(transcriptions))
        .with_state(state.clone());

    let addr = "0.0.0.0:8000";
    tracing::info!("Server starting on {}", addr);

    let listener = tokio::net::TcpListener::bind(addr).await.unwrap();
    axum::serve(listener, app).await.unwrap();
}
