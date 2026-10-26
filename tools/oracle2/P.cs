using System.Text.Json;
foreach (var d in new double[]{1e15,1e16,1.5e16,123456789012345678,1e20,1e21,1e-4,1.234e-5,-0.0,double.Epsilon,1.7976931348623157e308, 12345.678})
  Console.Write(JsonSerializer.Serialize(d)+" | "+d.ToString()+" ;; ");
Console.WriteLine();
foreach (var f in new float[]{1e7f,1e8f,3.4e38f,1.5e-5f,16777217f, 0.3f})
  Console.Write(JsonSerializer.Serialize(f)+" ;; ");
Console.WriteLine(JsonSerializer.Serialize(new string(new char[]{(char)0x2028,(char)0x7f,(char)0xa0,(char)9,(char)13,(char)8,(char)12,(char)0})));
Console.WriteLine(JsonSerializer.Serialize(new Dictionary<string,int>{{"b",1},{"a",2}}));
