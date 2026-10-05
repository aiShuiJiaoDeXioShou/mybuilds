import 'package:flutter/material.dart';

void main() {
  runApp(const MainApp());
}

class MainApp extends StatelessWidget {
  const MainApp({super.key});

  @override
  Widget build(BuildContext context) {
    return const MaterialApp(
      home: Scaffold(
        body: Center(
          // 渠道由流水线作为单个编译定义传入，不解释为代码。
          child: Text(String.fromEnvironment('MYBUILDS_CHANNEL', defaultValue: 'internal')),
        ),
      ),
    );
  }
}
