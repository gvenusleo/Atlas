import 'package:file_picker/file_picker.dart';

/// Opens the platform directory picker for a new workspace.
class const DirectoryPickerService() {
  /// Returns the selected path, or null when the dialog is dismissed.
  Future<String?> pick() => FilePicker.getDirectoryPath();
}
