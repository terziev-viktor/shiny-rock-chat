import 'package:flutter/material.dart';

void main() {
  runApp(const ShinyRockChatApp());
}

class ShinyRockChatApp extends StatelessWidget {
  const ShinyRockChatApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Shiny Rock Chat',
      home: const Scaffold(),
    );
  }
}
